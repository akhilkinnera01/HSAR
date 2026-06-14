package proxy

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	hsarv1 "github.com/hsar-org/hsar/gen/go/hsar/v1"
	"github.com/hsar-org/hsar/internal/config"
	"github.com/hsar-org/hsar/internal/policy"
)

// InlineSignalClient performs synchronous signal inference within a budget.
type InlineSignalClient interface {
	InlineGetSignals(ctx context.Context, tenantID, requestID, text string) (*hsarv1.SignalFrame, error)
	Evaluator() *policy.Evaluator
}

func WithInlineGovernance(cfg config.Config, client InlineSignalClient, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			slog.Error("failed_to_read_body", "error", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		r.Body.Close()

		reqID := r.Header.Get("X-Request-ID")
		conversationID := r.Header.Get("X-Conversation-ID")
		if conversationID == "" {
			conversationID = reqID
		}
		tenantID := "default-tenant"
		if tid, ok := TenantFromContext(r.Context()); ok {
			tenantID = tid
		}

		originalBody := append([]byte(nil), bodyBytes...)
		forwardBody := bodyBytes

		if !ShouldEnforce(cfg, reqID) || client == nil || client.Evaluator() == nil {
			r.Body = io.NopCloser(bytes.NewBuffer(forwardBody))
			next.ServeHTTP(w, r)
			return
		}

		budgetMs := cfg.InlineBudgetMs
		if budgetMs <= 0 {
			budgetMs = 30
		}
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(budgetMs)*time.Millisecond)
		defer cancel()

		sf, err := client.InlineGetSignals(ctx, tenantID, reqID, string(bodyBytes))
		if err != nil {
			slog.Warn("inline_fail_open", "trace_id", reqID, "reason", "signal_error", "error", err)
			r.Body = io.NopCloser(bytes.NewBuffer(originalBody))
			next.ServeHTTP(w, r)
			return
		}

		evaluator := client.Evaluator()
		decision := evaluator.Evaluate(tenantID, conversationID, sf)
		trace := policy.BuildTrace(tenantID, reqID, evaluator.Policy, decision)

		if sf.GetAbstain() {
			policy.LogTrace(trace, false)
			r.Body = io.NopCloser(bytes.NewBuffer(originalBody))
			next.ServeHTTP(w, r)
			return
		}

		if !ShouldApplyEnforcement(decision.ActionsApplied) {
			policy.LogTrace(trace, false)
			r.Body = io.NopCloser(bytes.NewBuffer(originalBody))
			next.ServeHTTP(w, r)
			return
		}

		result, err := ApplyActions(originalBody, decision.ActionsApplied)
		if err != nil {
			slog.Warn("inline_fail_open", "trace_id", reqID, "reason", "enforce_error", "error", err)
			r.Body = io.NopCloser(bytes.NewBuffer(originalBody))
			next.ServeHTTP(w, r)
			return
		}

		policy.LogTrace(trace, !result.ShortCircuit && result.Mutated || result.ShortCircuit)

		if result.ShortCircuit {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(result.StatusCode)
			_, _ = w.Write(result.ResponseBody)
			return
		}

		if result.Mutated {
			forwardBody = result.Body
		} else {
			forwardBody = originalBody
		}

		r.Body = io.NopCloser(bytes.NewBuffer(forwardBody))
		next.ServeHTTP(w, r)
	})
}
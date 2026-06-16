package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	hsarv1 "github.com/hsar-org/hsar/gen/go/hsar/v1"
	"github.com/hsar-org/hsar/internal/config"
	"github.com/hsar-org/hsar/internal/policy"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

		evaluator := client.Evaluator()

		sf, err := client.InlineGetSignals(ctx, tenantID, reqID, string(bodyBytes))
		if err != nil {
			reason := "signal_error"
			if isBudgetExceeded(err) {
				reason = "budget"
			}
			slog.Warn("inline_fail_open", "trace_id", reqID, "reason", reason, "error", err)
			logFailOpenPassthroughTrace(evaluator, tenantID, reqID)
			r.Body = io.NopCloser(bytes.NewBuffer(originalBody))
			next.ServeHTTP(w, r)
			return
		}

		decision, policyErr := evaluateSafe(evaluator, tenantID, conversationID, sf)
		if policyErr != nil {
			slog.Warn("inline_fail_open", "trace_id", reqID, "reason", "policy_error", "error", policyErr)
			logFailOpenPassthroughTrace(evaluator, tenantID, reqID)
			r.Body = io.NopCloser(bytes.NewBuffer(originalBody))
			next.ServeHTTP(w, r)
			return
		}
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

func isBudgetExceeded(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if st, ok := status.FromError(err); ok && st.Code() == codes.DeadlineExceeded {
		return true
	}
	return false
}

func evaluateSafe(evaluator *policy.Evaluator, tenantID, conversationID string, sf *hsarv1.SignalFrame) (decision policy.Decision, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("policy panic: %v", r)
		}
	}()
	decision = evaluator.Evaluate(tenantID, conversationID, sf)
	return decision, nil
}

func logFailOpenPassthroughTrace(evaluator *policy.Evaluator, tenantID, reqID string) {
	if evaluator == nil {
		return
	}
	decision := policy.Decision{
		DecisionID:  uuid.NewString(),
		Passthrough: true,
		ActionsApplied: []*hsarv1.ActionApplied{{
			Type: hsarv1.ActionType_ACTION_PASSTHROUGH,
		}},
		StabilityState: hsarv1.StabilityState_STATE_NORMAL,
	}
	trace := policy.BuildTrace(tenantID, reqID, evaluator.Policy, decision)
	policy.LogTrace(trace, false)
}
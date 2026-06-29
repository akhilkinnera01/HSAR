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
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/grpc/codes"
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

		bag, hasBag := TelemetryFromContext(r.Context())
		mode := string(cfg.Mode)
		if hasBag && bag.Mode != "" {
			mode = bag.Mode
		}

		originalBody := append([]byte(nil), bodyBytes...)
		forwardBody := bodyBytes

		inlineStart := time.Now()
		recordInline := func() {
			if hasBag && bag.Metrics != nil {
				bag.Metrics.InlineDuration.WithLabelValues(mode).Observe(time.Since(inlineStart).Seconds())
			}
		}

		if IsKillSwitchActive(cfg) && WouldEnforceWithoutKillSwitch(cfg, reqID) {
			if hasBag && bag.Metrics != nil {
				bag.Metrics.KillSwitchPassthrough.Inc()
			}
			r.Body = io.NopCloser(bytes.NewBuffer(forwardBody))
			next.ServeHTTP(w, r)
			return
		}

		if !ShouldEnforce(cfg, reqID) || client == nil || client.Evaluator() == nil {
			r.Body = io.NopCloser(bytes.NewBuffer(forwardBody))
			next.ServeHTTP(w, r)
			return
		}

		if hasBag && bag.Record != nil {
			bag.Record.Inline = true
		}

		budgetMs := cfg.InlineBudgetMs
		if budgetMs <= 0 {
			budgetMs = 30
		}
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(budgetMs)*time.Millisecond)
		defer cancel()

		evaluator := client.Evaluator()

		if hasBag && bag.Tracer != nil {
			var signalSpan trace.Span
			ctx, signalSpan = bag.Tracer.Start(ctx, "hsar.signal.inline")
			defer signalSpan.End()
		}

		signalStart := time.Now()
		sf, err := client.InlineGetSignals(ctx, tenantID, reqID, string(bodyBytes))
		if hasBag && bag.Metrics != nil {
			bag.Metrics.InlineSignalDuration.WithLabelValues(mode).Observe(time.Since(signalStart).Seconds())
		}
		if err != nil {
			reason := "signal_error"
			if isBudgetExceeded(err) {
				reason = "budget"
			}
			recordFailOpen(hasBag, bag, mode, reason, reqID, err)
			logFailOpenPassthroughTrace(evaluator, tenantID, reqID)
			recordInline()
			r.Body = io.NopCloser(bytes.NewBuffer(originalBody))
			next.ServeHTTP(w, r)
			return
		}

		if hasBag && bag.Tracer != nil {
			var policySpan trace.Span
			ctx, policySpan = bag.Tracer.Start(ctx, "hsar.policy.evaluate")
			defer policySpan.End()
		}

		policyStart := time.Now()
		decision, policyErr := evaluateSafe(evaluator, tenantID, conversationID, sf)
		if hasBag && bag.Metrics != nil {
			bag.Metrics.PolicyDuration.WithLabelValues(mode).Observe(time.Since(policyStart).Seconds())
		}
		if policyErr != nil {
			recordFailOpen(hasBag, bag, mode, "policy_error", reqID, policyErr)
			logFailOpenPassthroughTrace(evaluator, tenantID, reqID)
			recordInline()
			r.Body = io.NopCloser(bytes.NewBuffer(originalBody))
			next.ServeHTTP(w, r)
			return
		}
		trace := policy.BuildTrace(tenantID, reqID, evaluator.Policy, decision)

		if sf.GetAbstain() {
			if hasBag && bag.Metrics != nil {
				bag.Metrics.AbstainTotal.Inc()
			}
			if hasBag && bag.Record != nil {
				bag.Record.Abstain = true
			}
			recordAction(hasBag, bag, decision, false)
			policy.LogTrace(trace, false)
			recordInline()
			r.Body = io.NopCloser(bytes.NewBuffer(originalBody))
			next.ServeHTTP(w, r)
			return
		}

		if !ShouldApplyEnforcement(decision.ActionsApplied) {
			recordAction(hasBag, bag, decision, false)
			policy.LogTrace(trace, false)
			recordInline()
			r.Body = io.NopCloser(bytes.NewBuffer(originalBody))
			next.ServeHTTP(w, r)
			return
		}

		result, err := ApplyActions(originalBody, decision.ActionsApplied)
		if err != nil {
			slog.Warn("inline_fail_open", "trace_id", reqID, "reason", "enforce_error", "error", err)
			recordInline()
			r.Body = io.NopCloser(bytes.NewBuffer(originalBody))
			next.ServeHTTP(w, r)
			return
		}

		enforceApplied := !result.ShortCircuit && result.Mutated || result.ShortCircuit
		recordAction(hasBag, bag, decision, enforceApplied)
		if hasBag && bag.Record != nil {
			bag.Record.EnforceApplied = enforceApplied
		}

		policy.LogTrace(trace, enforceApplied)

		if result.ShortCircuit {
			if hasBag && bag.Span != nil {
				bag.Span.AddEvent("hsar.short_circuit")
			}
			recordInline()
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

		recordInline()
		r.Body = io.NopCloser(bytes.NewBuffer(forwardBody))
		next.ServeHTTP(w, r)
	})
}

func recordFailOpen(hasBag bool, bag TelemetryBag, mode, reason, reqID string, err error) {
	slog.Warn("inline_fail_open", "trace_id", reqID, "reason", reason, "error", err)
	if !hasBag {
		return
	}
	if bag.Metrics != nil {
		bag.Metrics.FailOpenTotal.WithLabelValues(reason, mode).Inc()
	}
	if bag.Record != nil {
		bag.Record.FailOpen = true
		bag.Record.FailOpenReason = reason
	}
	if bag.Span != nil {
		bag.Span.AddEvent("hsar.fail_open", trace.WithAttributes(
			attribute.String("hsar.fail_open_reason", reason),
		))
		bag.Span.SetStatus(otelcodes.Error, reason)
	}
}

func recordAction(hasBag bool, bag TelemetryBag, decision policy.Decision, enforceApplied bool) {
	if !hasBag || bag.Metrics == nil || len(decision.ActionsApplied) == 0 {
		return
	}
	actionType := decision.ActionsApplied[0].GetType().String()
	actionType = trimActionPrefix(actionType)
	enforceLabel := "false"
	if enforceApplied {
		enforceLabel = "true"
	}
	bag.Metrics.ActionApplied.WithLabelValues(actionType, enforceLabel).Inc()
	if bag.Record != nil && bag.Record.ActionType == "" {
		bag.Record.ActionType = actionType
	}
}

func trimActionPrefix(s string) string {
	const prefix = "ACTION_"
	if len(s) > len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

func isBudgetExceeded(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if st, ok := grpcstatus.FromError(err); ok && st.Code() == codes.DeadlineExceeded {
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
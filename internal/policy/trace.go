package policy

import (
	"log/slog"
	"time"

	hsarv1 "github.com/hsar-org/hsar/gen/go/hsar/v1"
)

// BuildTrace constructs a PolicyTrace proto from inputs.
func BuildTrace(tenantID, requestID string, p Policy, d Decision) *hsarv1.PolicyTrace {
	actions := make([]*hsarv1.ActionApplied, len(d.ActionsApplied))
	copy(actions, d.ActionsApplied)

	return &hsarv1.PolicyTrace{
		TenantId:       tenantID,
		RequestId:      requestID,
		TimestampMs:    time.Now().UnixMilli(),
		PolicyId:       p.PolicyID,
		PolicyVersion:  p.PolicyVersion,
		DecisionId:     d.DecisionID,
		ActionsApplied: actions,
		StabilityState: d.StabilityState,
	}
}

// LogTrace emits structured policy_trace log (privacy-safe).
func LogTrace(trace *hsarv1.PolicyTrace) {
	actions := make([]map[string]string, 0, len(trace.GetActionsApplied()))
	for _, a := range trace.GetActionsApplied() {
		actions = append(actions, map[string]string{
			"type":   a.GetType().String(),
			"detail": a.GetDetail(),
		})
	}

	slog.Info("policy_trace",
		"trace_id", trace.GetRequestId(),
		"tenant_id", trace.GetTenantId(),
		"decision_id", trace.GetDecisionId(),
		"policy_id", trace.GetPolicyId(),
		"policy_version", trace.GetPolicyVersion(),
		"stability_state", trace.GetStabilityState().String(),
		"actions", actions,
	)
}
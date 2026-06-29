package policy_test

import (
	"path/filepath"
	"testing"

	hsarv1 "github.com/hsar-org/hsar/gen/go/hsar/v1"
	"github.com/hsar-org/hsar/internal/policy"
)

func loadTestPolicy(t *testing.T) policy.Policy {
	t.Helper()
	path := filepath.Join("..", "..", "policies", "standard-safety-policy.yaml")
	p, err := policy.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return p
}

func frameWithRisk(value float32) *hsarv1.SignalFrame {
	return &hsarv1.SignalFrame{
		Abstain: false,
		Signals: []*hsarv1.Signal{{Name: "failure_risk", Value: value}},
	}
}

func TestEvaluateProducesTraceFields(t *testing.T) {
	p := loadTestPolicy(t)
	st := policy.ConversationState{
		StabilityState:   hsarv1.StabilityState_STATE_NORMAL,
		MatchedRuleIndex: -1,
	}
	decision, _ := policy.EvaluatePure(frameWithRisk(0.85), st, p)

	if decision.DecisionID == "" {
		t.Fatal("expected decision_id")
	}
	if decision.StabilityState != hsarv1.StabilityState_STATE_ACTIVE {
		t.Fatalf("expected ACTIVE, got %v", decision.StabilityState)
	}
	if len(decision.ActionsApplied) == 0 {
		t.Fatal("expected actions")
	}
	if decision.ActionsApplied[0].GetType() != hsarv1.ActionType_ACTION_INJECT_SYSTEM_CONTEXT {
		t.Fatalf("expected INJECT_SYSTEM_CONTEXT for 0.85 risk, got %v", decision.ActionsApplied[0].GetType())
	}
}

func TestAbstainYieldsPassthrough(t *testing.T) {
	p := loadTestPolicy(t)
	st := policy.ConversationState{StabilityState: hsarv1.StabilityState_STATE_NORMAL}
	frame := &hsarv1.SignalFrame{Abstain: true}
	decision, next := policy.EvaluatePure(frame, st, p)

	if !decision.Passthrough {
		t.Fatal("expected passthrough")
	}
	if next.StabilityState != hsarv1.StabilityState_STATE_NORMAL {
		t.Fatal("state should be unchanged on abstain")
	}
}

func TestMonotonicActionStrength(t *testing.T) {
	p := policy.Policy{
		PolicyID: "test", PolicyVersion: "v1", CooldownRequests: 0,
		Rules: []policy.PolicyRule{
			{Signal: "failure_risk", EnterThreshold: 0.8, ExitThreshold: 0.6, Action: "INJECT_SYSTEM_CONTEXT"},
			{Signal: "failure_risk", EnterThreshold: 0.5, ExitThreshold: 0.3, Action: "DAMPEN_VERBOSITY"},
		},
	}
	st := policy.ConversationState{
		StabilityState:   hsarv1.StabilityState_STATE_NORMAL,
		MatchedRuleIndex: -1,
	}
	risks := []float32{0.55, 0.65, 0.85}
	prev := -1
	for _, r := range risks {
		st.StabilityState = hsarv1.StabilityState_STATE_NORMAL
		st.MatchedRuleIndex = -1
		decision, _ := policy.EvaluatePure(frameWithRisk(r), st, p)
		if len(decision.ActionsApplied) == 0 {
			t.Fatalf("risk %v: no actions", r)
		}
		strength := policy.ActionStrength(decision.ActionsApplied[0].GetType())
		if strength < prev {
			t.Fatalf("risk %v: weaker action %d after %d", r, strength, prev)
		}
		prev = strength
	}
}

func TestRulePrecedenceHighestThresholdFirst(t *testing.T) {
	p := loadTestPolicy(t)
	st := policy.ConversationState{
		StabilityState:   hsarv1.StabilityState_STATE_NORMAL,
		MatchedRuleIndex: -1,
	}

	decision, _ := policy.EvaluatePure(frameWithRisk(0.95), st, p)
	if decision.ActionsApplied[0].GetType() != hsarv1.ActionType_ACTION_DAMPEN_VERBOSITY {
		t.Fatalf("0.95 risk: want DAMPEN_VERBOSITY, got %v", decision.ActionsApplied[0].GetType())
	}

	st = policy.ConversationState{StabilityState: hsarv1.StabilityState_STATE_NORMAL, MatchedRuleIndex: -1}
	decision, _ = policy.EvaluatePure(frameWithRisk(0.80), st, p)
	if decision.ActionsApplied[0].GetType() != hsarv1.ActionType_ACTION_INJECT_SYSTEM_CONTEXT {
		t.Fatalf("0.80 risk: want INJECT_SYSTEM_CONTEXT, got %v", decision.ActionsApplied[0].GetType())
	}
}

func TestBelowThresholdYieldsPassthrough(t *testing.T) {
	p := loadTestPolicy(t)
	st := policy.ConversationState{
		StabilityState:   hsarv1.StabilityState_STATE_NORMAL,
		MatchedRuleIndex: -1,
	}
	decision, next := policy.EvaluatePure(frameWithRisk(0.10), st, p)

	if !decision.Passthrough {
		t.Fatal("expected passthrough below all enter thresholds")
	}
	if next.StabilityState != hsarv1.StabilityState_STATE_NORMAL {
		t.Fatalf("state = %v, want NORMAL", next.StabilityState)
	}
}

func TestStickyRuleDuringCooldown(t *testing.T) {
	p := policy.Policy{
		PolicyID: "sticky", PolicyVersion: "v1", CooldownRequests: 2,
		Rules: []policy.PolicyRule{{
			Signal: "failure_risk", EnterThreshold: 0.75, ExitThreshold: 0.55,
			Action: "INJECT_SYSTEM_CONTEXT",
		}},
	}
	st := policy.ConversationState{
		StabilityState:   hsarv1.StabilityState_STATE_NORMAL,
		MatchedRuleIndex: -1,
	}
	_, st = policy.EvaluatePure(frameWithRisk(0.9), st, p)
	if st.StabilityState != hsarv1.StabilityState_STATE_ACTIVE {
		t.Fatalf("expected ACTIVE, got %v", st.StabilityState)
	}

	_, st = policy.EvaluatePure(frameWithRisk(0.4), st, p)
	if st.StabilityState != hsarv1.StabilityState_STATE_COOLDOWN &&
		st.StabilityState != hsarv1.StabilityState_STATE_ACTIVE {
		t.Fatalf("expected COOLDOWN or ACTIVE during cooldown, got %v", st.StabilityState)
	}
	if st.MatchedRuleIndex != 0 {
		t.Fatalf("MatchedRuleIndex = %d, want 0", st.MatchedRuleIndex)
	}
}

func TestBoundedActionEnum(t *testing.T) {
	p := loadTestPolicy(t)
	st := policy.ConversationState{StabilityState: hsarv1.StabilityState_STATE_NORMAL}
	for _, r := range []float32{0.1, 0.8, 0.95} {
		decision, _ := policy.EvaluatePure(frameWithRisk(r), st, p)
		for _, a := range decision.ActionsApplied {
			if policy.ActionStrength(a.GetType()) < 0 {
				t.Fatalf("invalid action type %v", a.GetType())
			}
		}
	}
}

func TestBuildTraceHasPolicyMetadata(t *testing.T) {
	p := loadTestPolicy(t)
	st := policy.ConversationState{StabilityState: hsarv1.StabilityState_STATE_NORMAL}
	decision, _ := policy.EvaluatePure(frameWithRisk(0.9), st, p)
	trace := policy.BuildTrace("tenant-a", "req-1", p, decision)

	if trace.GetPolicyId() != "standard-safety-policy" {
		t.Fatalf("policy_id = %q", trace.GetPolicyId())
	}
	if trace.GetDecisionId() == "" {
		t.Fatal("missing decision_id")
	}
}
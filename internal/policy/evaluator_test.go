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
	st := policy.ConversationState{StabilityState: hsarv1.StabilityState_STATE_NORMAL}
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
	if decision.ActionsApplied[0].GetType() == hsarv1.ActionType_ACTION_PASSTHROUGH {
		t.Fatal("expected non-passthrough action for high risk")
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
			{Signal: "failure_risk", EnterThreshold: 0.5, ExitThreshold: 0.3, Action: "DAMPEN_VERBOSITY"},
			{Signal: "failure_risk", EnterThreshold: 0.8, ExitThreshold: 0.6, Action: "INJECT_SYSTEM_CONTEXT"},
		},
	}
	st := policy.ConversationState{StabilityState: hsarv1.StabilityState_STATE_NORMAL}
	risks := []float32{0.55, 0.65, 0.85}
	prev := -1
	for _, r := range risks {
		decision, _ := policy.EvaluatePure(frameWithRisk(r), st, p)
		if len(decision.ActionsApplied) == 0 {
			t.Fatalf("risk %v: no actions", r)
		}
		strength := policy.ActionStrength(decision.ActionsApplied[0].GetType())
		if strength < prev {
			t.Fatalf("risk %v: weaker action %d after %d", r, strength, prev)
		}
		prev = strength
		st.StabilityState = hsarv1.StabilityState_STATE_NORMAL
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
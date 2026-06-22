package policy_test

import (
	"fmt"
	"sync"
	"testing"

	hsarv1 "github.com/hsar-org/hsar/gen/go/hsar/v1"
	"github.com/hsar-org/hsar/internal/policy"
)

func TestAntiFlapOscillatingSignal(t *testing.T) {
	p := policy.Policy{
		PolicyID:         "flap-test",
		PolicyVersion:    "v1",
		CooldownRequests: 3,
		Rules: []policy.PolicyRule{{
			Signal:         "failure_risk",
			EnterThreshold: 0.75,
			ExitThreshold:  0.55,
			Action:         "INJECT_SYSTEM_CONTEXT",
		}},
	}

	st := policy.ConversationState{
		StabilityState:   hsarv1.StabilityState_STATE_NORMAL,
		MatchedRuleIndex: -1,
	}
	sequence := []float32{0.80, 0.60, 0.80, 0.60, 0.80, 0.60}

	entries := 0
	prev := st.StabilityState
	for _, risk := range sequence {
		var d policy.Decision
		d, st = policy.EvaluatePure(frameWithRisk(risk), st, p)
		_ = d
		if prev == hsarv1.StabilityState_STATE_NORMAL &&
			st.StabilityState != hsarv1.StabilityState_STATE_NORMAL {
			entries++
		}
		prev = st.StabilityState
	}

	if entries > 1 {
		t.Fatalf("anti-flap violated: %d NORMAL→ACTIVE entries, want <=1", entries)
	}
}

func TestCooldownHoldsActive(t *testing.T) {
	p := policy.Policy{
		PolicyID:         "cooldown-test",
		PolicyVersion:    "v1",
		CooldownRequests: 2,
		Rules: []policy.PolicyRule{{
			Signal:         "failure_risk",
			EnterThreshold: 0.75,
			ExitThreshold:  0.55,
			Action:         "INJECT_SYSTEM_CONTEXT",
		}},
	}

	st := policy.ConversationState{
		StabilityState:   hsarv1.StabilityState_STATE_NORMAL,
		MatchedRuleIndex: -1,
	}
	_, st = policy.EvaluatePure(frameWithRisk(0.9), st, p)
	if st.StabilityState != hsarv1.StabilityState_STATE_ACTIVE {
		t.Fatalf("expected ACTIVE after high risk, got %v", st.StabilityState)
	}

	_, st = policy.EvaluatePure(frameWithRisk(0.4), st, p)
	if st.StabilityState != hsarv1.StabilityState_STATE_COOLDOWN &&
		st.StabilityState != hsarv1.StabilityState_STATE_ACTIVE {
		t.Fatalf("expected COOLDOWN or ACTIVE during cooldown, got %v", st.StabilityState)
	}
}

func TestStateStoreConcurrentUpdate(t *testing.T) {
	t.Parallel()

	store := policy.NewStateStore()
	p := policy.Policy{
		PolicyID:      "concurrent",
		PolicyVersion: "v1",
		Rules: []policy.PolicyRule{{
			Signal:         "failure_risk",
			EnterThreshold: 0.5,
			ExitThreshold:  0.3,
			Action:         "PASSTHROUGH",
		}},
	}
	eval := policy.NewEvaluator(p)

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tenant := fmt.Sprintf("tenant-%d", n%20)
			conv := fmt.Sprintf("conv-%d", n)
			frame := frameWithRisk(float32(n%100) / 100)
			eval.Evaluate(tenant, conv, frame)
			_ = store.Get(tenant, conv)
		}(i)
	}
	wg.Wait()
}
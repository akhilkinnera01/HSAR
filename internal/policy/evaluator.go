package policy

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	hsarv1 "github.com/hsar-org/hsar/gen/go/hsar/v1"
)

// Decision is the ephemeral policy evaluation result.
type Decision struct {
	DecisionID     string
	ActionsApplied []*hsarv1.ActionApplied
	StabilityState hsarv1.StabilityState
	Passthrough    bool
}

// Evaluator runs counterfactual policy evaluation.
type Evaluator struct {
	Policy Policy
	Store  *StateStore
}

func NewEvaluator(p Policy) *Evaluator {
	return &Evaluator{Policy: p, Store: NewStateStore()}
}

// ActionStrength returns monotonic ordering for property tests.
func ActionStrength(t hsarv1.ActionType) int {
	switch t {
	case hsarv1.ActionType_ACTION_PASSTHROUGH:
		return 0
	case hsarv1.ActionType_ACTION_DAMPEN_VERBOSITY:
		return 1
	case hsarv1.ActionType_ACTION_INJECT_SYSTEM_CONTEXT:
		return 2
	case hsarv1.ActionType_ACTION_ESCALATE_HUMAN:
		return 3
	case hsarv1.ActionType_ACTION_BLOCK_UNSAFE:
		return 4
	default:
		return -1
	}
}

func signalValue(frame *hsarv1.SignalFrame, name string) (float32, bool) {
	for _, s := range frame.GetSignals() {
		if s.GetName() == name {
			return s.GetValue(), true
		}
	}
	return 0, false
}

func isFSMActive(state hsarv1.StabilityState) bool {
	switch state {
	case hsarv1.StabilityState_STATE_ACTIVE,
		hsarv1.StabilityState_STATE_COOLDOWN,
		hsarv1.StabilityState_STATE_HYSTERESIS_ENTRY:
		return true
	default:
		return false
	}
}

// selectRule picks the governing rule: sticky MatchedRuleIndex during FSM,
// otherwise first ordered rule whose signal value meets enter_threshold.
func selectRule(p Policy, frame *hsarv1.SignalFrame, st ConversationState) (PolicyRule, int, bool) {
	if isFSMActive(st.StabilityState) && st.MatchedRuleIndex >= 0 && st.MatchedRuleIndex < len(p.Rules) {
		rule := p.Rules[st.MatchedRuleIndex]
		if _, ok := signalValue(frame, rule.Signal); ok {
			return rule, st.MatchedRuleIndex, true
		}
	}

	for i, rule := range p.Rules {
		val, ok := signalValue(frame, rule.Signal)
		if ok && val >= rule.EnterThreshold {
			return rule, i, true
		}
	}
	return PolicyRule{}, -1, false
}

func actionFromRule(rule PolicyRule) (hsarv1.ActionType, string) {
	name := normalizeAction(rule.Action)
	switch name {
	case "ACTION_DAMPEN_VERBOSITY":
		return hsarv1.ActionType_ACTION_DAMPEN_VERBOSITY, detailFromParams(rule.Params)
	case "ACTION_INJECT_SYSTEM_CONTEXT":
		return hsarv1.ActionType_ACTION_INJECT_SYSTEM_CONTEXT, detailFromParams(rule.Params)
	case "ACTION_ESCALATE_HUMAN":
		return hsarv1.ActionType_ACTION_ESCALATE_HUMAN, detailFromParams(rule.Params)
	case "ACTION_BLOCK_UNSAFE":
		return hsarv1.ActionType_ACTION_BLOCK_UNSAFE, detailFromParams(rule.Params)
	default:
		return hsarv1.ActionType_ACTION_PASSTHROUGH, ""
	}
}

func detailFromParams(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}
	if d, ok := params["detail"]; ok {
		return d
	}
	parts := make([]string, 0, len(params))
	for k, v := range params {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, ";")
}

// EvaluatePure is a pure function for tests (no store mutation).
func EvaluatePure(frame *hsarv1.SignalFrame, st ConversationState, p Policy) (Decision, ConversationState) {
	decision := Decision{
		DecisionID:     uuid.NewString(),
		StabilityState: st.StabilityState,
		Passthrough:    true,
		ActionsApplied: []*hsarv1.ActionApplied{{
			Type:   hsarv1.ActionType_ACTION_PASSTHROUGH,
			Detail: "",
		}},
	}

	if frame.GetAbstain() {
		return decision, st
	}

	rule, idx, ok := selectRule(p, frame, st)
	if !ok {
		return decision, st
	}

	val, _ := signalValue(frame, rule.Signal)
	next := st
	if idx >= 0 {
		next.MatchedRuleIndex = idx
	}
	transitionFSM(&next, val, rule, p.CooldownRequests)

	decision.StabilityState = next.StabilityState
	decision.Passthrough = false

	if next.StabilityState == hsarv1.StabilityState_STATE_ACTIVE ||
		next.StabilityState == hsarv1.StabilityState_STATE_COOLDOWN ||
		next.StabilityState == hsarv1.StabilityState_STATE_HYSTERESIS_ENTRY {
		atype, detail := actionFromRule(rule)
		decision.ActionsApplied = []*hsarv1.ActionApplied{{Type: atype, Detail: detail}}
	}

	return decision, next
}

// Evaluate updates store and returns decision for a live request.
func (e *Evaluator) Evaluate(tenantID, conversationID string, frame *hsarv1.SignalFrame) Decision {
	var decision Decision
	e.Store.Update(tenantID, conversationID, func(st *ConversationState) {
		var next ConversationState
		decision, next = EvaluatePure(frame, *st, e.Policy)
		*st = next
	})
	return decision
}
package proxy

import (
	"encoding/json"
	"strconv"
	"strings"

	hsarv1 "github.com/hsar-org/hsar/gen/go/hsar/v1"
)

// EnforcementResult is the outcome of applying policy actions to a request body.
type EnforcementResult struct {
	Body         []byte
	Mutated      bool
	ShortCircuit bool
	StatusCode   int
	ResponseBody []byte
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionBody struct {
	Messages  []chatMessage `json:"messages"`
	MaxTokens *int          `json:"max_tokens,omitempty"`
	Stream    bool          `json:"stream,omitempty"`
}

// ApplyActions mutates the request body or returns a short-circuit response.
func ApplyActions(body []byte, actions []*hsarv1.ActionApplied) (EnforcementResult, error) {
	if len(actions) == 0 {
		return EnforcementResult{Body: body}, nil
	}

	action := actions[0].GetType()
	detail := actions[0].GetDetail()

	switch action {
	case hsarv1.ActionType_ACTION_PASSTHROUGH:
		return EnforcementResult{Body: body}, nil
	case hsarv1.ActionType_ACTION_INJECT_SYSTEM_CONTEXT:
		out, err := injectSystemContext(body, detail)
		if err != nil {
			return EnforcementResult{}, err
		}
		return EnforcementResult{Body: out, Mutated: true}, nil
	case hsarv1.ActionType_ACTION_DAMPEN_VERBOSITY:
		out, err := dampenVerbosity(body, detail)
		if err != nil {
			return EnforcementResult{}, err
		}
		return EnforcementResult{Body: out, Mutated: true}, nil
	case hsarv1.ActionType_ACTION_ESCALATE_HUMAN:
		return EnforcementResult{
			ShortCircuit: true,
			StatusCode:   200,
			ResponseBody: escalationResponse(),
		}, nil
	case hsarv1.ActionType_ACTION_BLOCK_UNSAFE:
		return EnforcementResult{
			ShortCircuit: true,
			StatusCode:   400,
			ResponseBody: blockResponse(),
		}, nil
	default:
		return EnforcementResult{Body: body}, nil
	}
}

func injectSystemContext(body []byte, detail string) ([]byte, error) {
	var req chatCompletionBody
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	content := detail
	if content == "" {
		content = "Apply calm, safety-focused guidance."
	}
	sys := chatMessage{Role: "system", Content: content}
	req.Messages = append([]chatMessage{sys}, req.Messages...)
	return json.Marshal(req)
}

func dampenVerbosity(body []byte, detail string) ([]byte, error) {
	var req chatCompletionBody
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if maxTok := parseMaxTokens(detail); maxTok > 0 {
		req.MaxTokens = &maxTok
	}
	terse := chatMessage{Role: "system", Content: "Respond briefly and calmly."}
	req.Messages = append([]chatMessage{terse}, req.Messages...)
	return json.Marshal(req)
}

func parseMaxTokens(detail string) int {
	for _, part := range strings.Split(detail, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "max_tokens=") {
			n, err := strconv.Atoi(strings.TrimPrefix(part, "max_tokens="))
			if err == nil {
				return n
			}
		}
	}
	return 0
}

func escalationResponse() []byte {
	resp := map[string]any{
		"id":      "hsar-escalation",
		"object":  "chat.completion",
		"choices": []map[string]any{{
			"message": map[string]string{
				"role":    "assistant",
				"content": "A human agent will assist you shortly. Your request has been queued for review.",
			},
			"finish_reason": "stop",
		}},
	}
	b, _ := json.Marshal(resp)
	return b
}

func blockResponse() []byte {
	resp := map[string]any{
		"error": map[string]string{
			"message": "Request blocked by safety policy",
			"type":    "hsar_block",
			"code":    "policy_block",
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

// ShouldApplyEnforcement returns true when decision warrants request mutation.
func ShouldApplyEnforcement(decisionActions []*hsarv1.ActionApplied) bool {
	if len(decisionActions) == 0 {
		return false
	}
	t := decisionActions[0].GetType()
	return t != hsarv1.ActionType_ACTION_PASSTHROUGH &&
		t != hsarv1.ActionType_ACTION_TYPE_UNSPECIFIED
}


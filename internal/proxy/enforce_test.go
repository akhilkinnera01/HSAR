package proxy_test

import (
	"encoding/json"
	"testing"

	hsarv1 "github.com/hsar-org/hsar/gen/go/hsar/v1"
	"github.com/hsar-org/hsar/internal/proxy"
)

func TestApplyActionsInjectSystemContext(t *testing.T) {
	t.Parallel()

	body := []byte(`{"messages":[{"role":"user","content":"angry"}]}`)
	actions := []*hsarv1.ActionApplied{{
		Type:   hsarv1.ActionType_ACTION_INJECT_SYSTEM_CONTEXT,
		Detail: "tone=calm",
	}}

	result, err := proxy.ApplyActions(body, actions)
	if err != nil {
		t.Fatalf("ApplyActions: %v", err)
	}
	if !result.Mutated {
		t.Fatal("expected mutation")
	}

	var parsed struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(result.Body, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Messages) < 2 {
		t.Fatalf("messages len = %d", len(parsed.Messages))
	}
	if parsed.Messages[0].Role != "system" || parsed.Messages[0].Content != "tone=calm" {
		t.Fatalf("system message = %+v", parsed.Messages[0])
	}
}

func TestApplyActionsDampenVerbosity(t *testing.T) {
	t.Parallel()

	body := []byte(`{"messages":[{"role":"user","content":"angry"}]}`)
	actions := []*hsarv1.ActionApplied{{
		Type:   hsarv1.ActionType_ACTION_DAMPEN_VERBOSITY,
		Detail: "max_tokens=128",
	}}

	result, err := proxy.ApplyActions(body, actions)
	if err != nil {
		t.Fatalf("ApplyActions: %v", err)
	}
	if !result.Mutated {
		t.Fatal("expected mutation")
	}

	var parsed struct {
		Messages  []map[string]string `json:"messages"`
		MaxTokens *int                `json:"max_tokens"`
	}
	if err := json.Unmarshal(result.Body, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.MaxTokens == nil || *parsed.MaxTokens != 128 {
		t.Fatalf("max_tokens = %v", parsed.MaxTokens)
	}
	if parsed.Messages[0]["role"] != "system" {
		t.Fatal("expected terse system message")
	}
}

func TestApplyActionsEscalateShortCircuit(t *testing.T) {
	t.Parallel()

	body := []byte(`{"messages":[{"role":"user","content":"help"}]}`)
	actions := []*hsarv1.ActionApplied{{
		Type: hsarv1.ActionType_ACTION_ESCALATE_HUMAN,
	}}

	result, err := proxy.ApplyActions(body, actions)
	if err != nil {
		t.Fatalf("ApplyActions: %v", err)
	}
	if !result.ShortCircuit || result.StatusCode != 200 {
		t.Fatalf("result = %+v", result)
	}
}

func TestApplyActionsBlockShortCircuit(t *testing.T) {
	t.Parallel()

	body := []byte(`{"messages":[{"role":"user","content":"bad"}]}`)
	actions := []*hsarv1.ActionApplied{{
		Type: hsarv1.ActionType_ACTION_BLOCK_UNSAFE,
	}}

	result, err := proxy.ApplyActions(body, actions)
	if err != nil {
		t.Fatalf("ApplyActions: %v", err)
	}
	if !result.ShortCircuit || result.StatusCode != 400 {
		t.Fatalf("result = %+v", result)
	}
}
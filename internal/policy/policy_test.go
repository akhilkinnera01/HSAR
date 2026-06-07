package policy_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hsar-org/hsar/internal/policy"
)

func TestLoadStandardPolicy(t *testing.T) {
	root := filepath.Join("..", "..")
	path := filepath.Join(root, "policies", "standard-safety-policy.yaml")
	p, err := policy.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.PolicyID != "standard-safety-policy" {
		t.Fatalf("policy_id = %q", p.PolicyID)
	}
	if len(p.Rules) < 1 {
		t.Fatal("expected rules")
	}
}

func TestValidateRejectsBadThresholds(t *testing.T) {
	p := policy.Policy{
		PolicyID:         "x",
		PolicyVersion:    "v1",
		CooldownRequests: 1,
		Rules: []policy.PolicyRule{{
			Signal:         "failure_risk",
			EnterThreshold: 0.5,
			ExitThreshold:  0.6,
			Action:         "PASSTHROUGH",
		}},
	}
	if err := p.Validate(); err == nil {
		t.Fatal("expected validation error for enter <= exit")
	}
}

func TestLoadMissingFileFails(t *testing.T) {
	_, err := policy.Load("/nonexistent/policy.yaml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadFromEnvPath(t *testing.T) {
	root := filepath.Join("..", "..")
	path := filepath.Join(root, "policies", "standard-safety-policy.yaml")
	t.Setenv("POLICY_PATH", path)
	p, err := policy.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.PolicyVersion != "v1.0.0" {
		t.Fatalf("version = %q", p.PolicyVersion)
	}
	_ = os.Unsetenv("POLICY_PATH")
}
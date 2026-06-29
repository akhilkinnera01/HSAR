package policy

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultPolicyPath = "policies/standard-safety-policy.yaml"

var validSignals = map[string]struct{}{
	"failure_risk": {},
}

var validActions = map[string]struct{}{
	"ACTION_PASSTHROUGH":           {},
	"ACTION_DAMPEN_VERBOSITY":        {},
	"ACTION_INJECT_SYSTEM_CONTEXT":   {},
	"ACTION_ESCALATE_HUMAN":          {},
	"ACTION_BLOCK_UNSAFE":            {},
}

// Policy is the loaded YAML governance artifact.
type Policy struct {
	PolicyID         string       `yaml:"policy_id"`
	PolicyVersion    string       `yaml:"policy_version"`
	CooldownRequests int          `yaml:"cooldown_requests"`
	Rules            []PolicyRule `yaml:"rules"`
}

// PolicyRule is an ordered when/then rule.
type PolicyRule struct {
	Signal         string            `yaml:"signal"`
	EnterThreshold float32           `yaml:"enter_threshold"`
	ExitThreshold  float32           `yaml:"exit_threshold"`
	Action         string            `yaml:"action"`
	Params         map[string]string `yaml:"params"`
}

// Load reads and validates policy from path or POLICY_PATH env.
func Load(path string) (Policy, error) {
	if path == "" {
		path = os.Getenv("POLICY_PATH")
	}
	if path == "" {
		path = defaultPolicyPath
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, fmt.Errorf("read policy %q: %w", path, err)
	}

	var p Policy
	if err := yaml.Unmarshal(raw, &p); err != nil {
		return Policy{}, fmt.Errorf("parse policy: %w", err)
	}
	if err := p.Validate(); err != nil {
		return Policy{}, err
	}
	return p, nil
}

// Validate checks policy invariants at startup.
func (p Policy) Validate() error {
	if p.PolicyID == "" {
		return fmt.Errorf("policy_id is required")
	}
	if p.PolicyVersion == "" {
		return fmt.Errorf("policy_version is required")
	}
	if p.CooldownRequests < 0 {
		return fmt.Errorf("cooldown_requests must be >= 0")
	}
	if len(p.Rules) == 0 {
		return fmt.Errorf("at least one rule is required")
	}
	for i, r := range p.Rules {
		if r.Signal == "" {
			return fmt.Errorf("rule %d: signal is required", i)
		}
		if _, ok := validSignals[r.Signal]; !ok {
			return fmt.Errorf("rule %d: unknown signal %q", i, r.Signal)
		}
		if r.EnterThreshold <= r.ExitThreshold {
			return fmt.Errorf("rule %d: enter_threshold must be > exit_threshold", i)
		}
		action := normalizeAction(r.Action)
		if _, ok := validActions[action]; !ok {
			return fmt.Errorf("rule %d: unknown action %q", i, r.Action)
		}
	}
	return nil
}

func normalizeAction(action string) string {
	action = strings.TrimSpace(action)
	if !strings.HasPrefix(action, "ACTION_") {
		action = "ACTION_" + action
	}
	return action
}
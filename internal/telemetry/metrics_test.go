package telemetry

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

var forbiddenMetricSubstrings = []string{
	"messages", "content", "text_payload", "prompt", "request_id", "tenant_id", "conversation_id",
}

func TestMetricCatalogRegisters(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	m.RequestDuration.WithLabelValues("shadow", "/v1/chat/completions").Observe(0.01)
	m.FailOpenTotal.WithLabelValues("budget", "enforce").Inc()
	m.AbstainTotal.Inc()
	m.KillSwitchPassthrough.Inc()
	m.PolicyDuration.WithLabelValues("enforce").Observe(0.001)

	n, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(n) < 5 {
		t.Fatalf("expected registered metrics, got %d families", len(n))
	}
}

func TestMetricLabelsDenylist(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	m.FailOpenTotal.WithLabelValues("budget", "enforce").Inc()
	m.ActionApplied.WithLabelValues("PASSTHROUGH", "false").Inc()

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		name := mf.GetName()
		for _, forbidden := range forbiddenMetricSubstrings {
			if strings.Contains(name, forbidden) {
				t.Errorf("metric name contains forbidden %q: %s", forbidden, name)
			}
		}
		checkLabelValues(t, mf, forbiddenMetricSubstrings)
	}
}

func checkLabelValues(t *testing.T, mf *dto.MetricFamily, forbidden []string) {
	t.Helper()
	for _, m := range mf.GetMetric() {
		for _, lp := range m.GetLabel() {
			val := strings.ToLower(lp.GetValue())
			for _, f := range forbidden {
				if strings.Contains(val, f) {
					t.Errorf("metric %s label %s=%q contains forbidden %q", mf.GetName(), lp.GetName(), lp.GetValue(), f)
				}
			}
		}
	}
}

func TestFailOpenReasonAllowlist(t *testing.T) {
	allowed := map[string]bool{"budget": true, "signal_error": true, "policy_error": true}
	for _, reason := range []string{"budget", "signal_error", "policy_error"} {
		if !allowed[reason] {
			t.Fatalf("missing allowed reason %s", reason)
		}
	}
	disallowed := []string{"abstain", "kill_switch", "enforce_error"}
	for _, reason := range disallowed {
		if allowed[reason] {
			t.Errorf("disallowed reason %q must not be fail-open", reason)
		}
	}
}
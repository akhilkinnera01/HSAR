package telemetry

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the Phase 5 Prometheus catalog per contracts/metrics.md.
type Metrics struct {
	RequestDuration      *prometheus.HistogramVec
	InlineDuration       *prometheus.HistogramVec
	ShadowSignalDuration *prometheus.HistogramVec
	InlineSignalDuration *prometheus.HistogramVec
	PolicyDuration       *prometheus.HistogramVec
	FailOpenTotal        *prometheus.CounterVec
	AbstainTotal         prometheus.Counter
	KillSwitchPassthrough prometheus.Counter
	ActionApplied        *prometheus.CounterVec
	PolicyStateTransition *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	m := &Metrics{
		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "hsar_request_duration_seconds",
			Help:    "End-to-end proxy handler duration.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}, []string{"mode", "path"}),
		InlineDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "hsar_inline_duration_seconds",
			Help:    "Inline governance wall time (0 if inline path skipped).",
			Buckets: []float64{0.001, 0.005, 0.01, 0.02, 0.03, 0.05, 0.1, 0.25},
		}, []string{"mode"}),
		ShadowSignalDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "hsar_shadow_signal_duration_seconds",
			Help:    "Async shadow perception duration.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.02, 0.03, 0.05, 0.1, 0.25},
		}, []string{"mode"}),
		InlineSignalDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "hsar_inline_signal_duration_seconds",
			Help:    "Inline perception gRPC duration on enforce path.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.02, 0.03, 0.05, 0.1, 0.25},
		}, []string{"mode"}),
		PolicyDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "hsar_policy_duration_seconds",
			Help:    "Policy Evaluate duration on inline enforce path.",
			Buckets: []float64{0.0005, 0.001, 0.002, 0.005, 0.01, 0.02, 0.05},
		}, []string{"mode"}),
		FailOpenTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "hsar_fail_open_total",
			Help: "Inline fail-open passthrough events (budget, signal_error, policy_error only).",
		}, []string{"reason", "mode"}),
		AbstainTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "hsar_abstain_total",
			Help: "Perception abstention outcomes (not fail-open).",
		}),
		KillSwitchPassthrough: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "hsar_kill_switch_passthrough_total",
			Help: "Kill switch skipped inline enforce passthrough.",
		}),
		ActionApplied: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "hsar_action_applied_total",
			Help: "Governance actions applied or counterfactual.",
		}, []string{"action_type", "enforce_applied"}),
		PolicyStateTransition: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "hsar_policy_state_transition_total",
			Help: "Policy FSM state transitions.",
		}, []string{"from_state", "to_state"}),
	}

	reg.MustRegister(
		m.RequestDuration,
		m.InlineDuration,
		m.ShadowSignalDuration,
		m.InlineSignalDuration,
		m.PolicyDuration,
		m.FailOpenTotal,
		m.AbstainTotal,
		m.KillSwitchPassthrough,
		m.ActionApplied,
		m.PolicyStateTransition,
	)
	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.Handler()
}

func ModeLabel(mode string) string {
	switch mode {
	case "shadow", "canary", "enforce":
		return mode
	default:
		return "shadow"
	}
}
package proxy

import (
	"context"

	"github.com/hsar-org/hsar/internal/config"
	"github.com/hsar-org/hsar/internal/telemetry"
	"go.opentelemetry.io/otel/trace"
)

type telemetryKey struct{}

// GovernanceRecord captures per-request governance outcomes for root span export.
type GovernanceRecord struct {
	Inline          bool
	FailOpen        bool
	FailOpenReason  string
	Abstain         bool
	ActionType      string
	EnforceApplied  bool
}

// TelemetryBag carries per-request observability handles.
type TelemetryBag struct {
	Metrics *telemetry.Metrics
	Tracer  trace.Tracer
	Mode    string
	Span    trace.Span
	Record  *GovernanceRecord
}

func WithTelemetryBag(ctx context.Context, bag TelemetryBag) context.Context {
	return context.WithValue(ctx, telemetryKey{}, bag)
}

func TelemetryFromContext(ctx context.Context) (TelemetryBag, bool) {
	bag, ok := ctx.Value(telemetryKey{}).(TelemetryBag)
	return bag, ok
}

func WouldEnforceWithoutKillSwitch(cfg config.Config, requestID string) bool {
	switch cfg.Mode {
	case config.ModeEnforce:
		return true
	case config.ModeCanary:
		return InCanaryCohort(requestID, cfg.CanaryPct)
	default:
		return false
	}
}
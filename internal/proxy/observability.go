package proxy

import (
	"net/http"
	"time"

	"github.com/hsar-org/hsar/internal/config"
	"github.com/hsar-org/hsar/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// WithObservability records request duration, root span, and governance attributes.
func WithObservability(cfg config.Config, m *telemetry.Metrics, tracer trace.Tracer, next http.Handler) http.Handler {
	mode := telemetry.ModeLabel(string(cfg.Mode))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &GovernanceRecord{}
		var span trace.Span
		ctx := r.Context()
		if tracer != nil {
			ctx, span = tracer.Start(ctx, "hsar.chat.completion")
			defer span.End()
		}

		if m != nil || tracer != nil {
			ctx = WithTelemetryBag(ctx, TelemetryBag{
				Metrics: m,
				Tracer:  tracer,
				Mode:    mode,
				Span:    span,
				Record:  rec,
			})
		}

		ww := &responseWrapper{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(ww, r.WithContext(ctx))

		tenantID := "default-tenant"
		if tid, ok := TenantFromContext(ctx); ok {
			tenantID = tid
		}
		traceID := r.Header.Get("X-Request-ID")
		duration := time.Since(start).Seconds()

		if m != nil {
			m.RequestDuration.WithLabelValues(mode, r.URL.Path).Observe(duration)
		}

		if span != nil {
			for _, attr := range telemetry.SpanAttrs(
				traceID, tenantID, mode,
				rec.Inline, rec.EnforceApplied, rec.FailOpen, rec.FailOpenReason,
				rec.ActionType, rec.Abstain,
			) {
				span.SetAttributes(attr)
			}
			span.SetAttributes(attribute.Int("http.status_code", ww.statusCode))
			if ww.statusCode >= 500 {
				span.SetStatus(codes.Error, http.StatusText(ww.statusCode))
			}
		}
	})
}
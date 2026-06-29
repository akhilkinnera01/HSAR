package telemetry

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/hsar-org/hsar/proxy"

// TracerProvider wraps OTel trace setup and shutdown.
type TracerProvider struct {
	provider *sdktrace.TracerProvider
	Tracer   trace.Tracer
}

type OTelConfig struct {
	Endpoint    string
	ServiceName string
	SampleRatio float64
	Enabled     bool
}

func LoadOTelConfig() OTelConfig {
	ratio := 1.0
	if v := os.Getenv("TELEMETRY_SAMPLE_RATIO"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 1 {
			ratio = f
		}
	}
	return OTelConfig{
		Endpoint:    envOr("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector:4317"),
		ServiceName: envOr("OTEL_SERVICE_NAME", "hsar-proxy"),
		SampleRatio: ratio,
		Enabled:     envOr("OTEL_ENABLED", "true") != "false",
	}
}

func InitTracer(ctx context.Context, cfg OTelConfig) (*TracerProvider, error) {
	if !cfg.Enabled {
		tp := &TracerProvider{Tracer: otel.Tracer(tracerName)}
		return tp, nil
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("otlp exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	otel.SetTracerProvider(provider)

	return &TracerProvider{
		provider: provider,
		Tracer:   provider.Tracer(tracerName),
	}, nil
}

func (tp *TracerProvider) Shutdown(ctx context.Context) error {
	if tp.provider == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return tp.provider.Shutdown(ctx)
}

// SpanAttrs builds allowlisted attributes for hsar.chat.completion.
func SpanAttrs(traceID, tenantID, mode string, inline, enforceApplied, failOpen bool, failOpenReason, actionType string, abstain bool) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("hsar.trace_id", traceID),
		attribute.String("hsar.tenant_id", tenantID),
		attribute.String("hsar.mode", mode),
		attribute.Bool("hsar.inline", inline),
		attribute.Bool("hsar.enforce_applied", enforceApplied),
		attribute.Bool("hsar.fail_open", failOpen),
		attribute.String("http.method", "POST"),
		attribute.String("http.route", "/v1/chat/completions"),
	}
	if failOpen && failOpenReason != "" {
		attrs = append(attrs, attribute.String("hsar.fail_open_reason", failOpenReason))
	}
	if actionType != "" {
		attrs = append(attrs, attribute.String("hsar.action_type", actionType))
	}
	if abstain {
		attrs = append(attrs, attribute.Bool("hsar.abstain", true))
	}
	return attrs
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
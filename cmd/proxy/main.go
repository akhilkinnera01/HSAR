package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	hsarv1 "github.com/hsar-org/hsar/gen/go/hsar/v1"
	"github.com/hsar-org/hsar/internal/config"
	"github.com/hsar-org/hsar/internal/engine"
	"github.com/hsar-org/hsar/internal/policy"
	"github.com/hsar-org/hsar/internal/proxy"
	"github.com/hsar-org/hsar/internal/telemetry"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config_load_failed", "error", err)
		os.Exit(1)
	}

	logger.Info("starting_hsar_proxy",
		"port", cfg.Port,
		"upstream", cfg.UpstreamBaseURL,
		"mode", cfg.Mode,
	)

	var metrics *telemetry.Metrics
	if cfg.MetricsEnabled {
		metrics = telemetry.NewMetrics(nil)
	}

	ctx := context.Background()
	otelCfg := telemetry.OTelConfig{
		Endpoint:    cfg.OTelEndpoint,
		ServiceName: cfg.OTelServiceName,
		SampleRatio: cfg.TelemetrySampleRatio,
		Enabled:     true,
	}
	tp, err := telemetry.InitTracer(ctx, otelCfg)
	if err != nil {
		slog.Warn("otel_init_failed", "error", err)
		tp = &telemetry.TracerProvider{Tracer: nil}
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			slog.Warn("otel_shutdown_failed", "error", err)
		}
	}()

	pol, err := policy.Load("")
	if err != nil {
		logger.Error("policy_load_failed", "error", err)
		os.Exit(1)
	}
	evaluator := policy.NewEvaluator(pol)
	modeLabel := telemetry.ModeLabel(string(cfg.Mode))
	if metrics != nil {
		evaluator.OnStateTransition = func(from, to hsarv1.StabilityState) {
			metrics.PolicyStateTransition.WithLabelValues(from.String(), to.String()).Inc()
		}
	}
	logger.Info("policy_loaded",
		"policy_id", pol.PolicyID,
		"policy_version", pol.PolicyVersion,
	)

	var sigClient *engine.Client
	sigClient, err = engine.NewClientFromEnv(evaluator)
	if err != nil {
		slog.Warn("signal_engine_disabled", "error", err)
		sigClient = nil
	} else {
		logger.Info("signal_engine_connected")
		if metrics != nil {
			sigClient.OnShadowAbstain = func() { metrics.AbstainTotal.Inc() }
		}
	}

	defer func() {
		if sigClient != nil {
			_ = sigClient.Close()
		}
	}()

	upstream, err := proxy.NewUpstream(cfg.UpstreamBaseURL)
	if err != nil {
		logger.Error("invalid_upstream_url", "error", err)
		os.Exit(1)
	}

	rateLimiter := proxy.NewRateLimiter(cfg)

	chatHandler := proxy.WithObservability(cfg, metrics, tp.Tracer,
		proxy.WithLogging(
			proxy.WithTraceID(
				proxy.WithRequestDeadline(15*time.Second,
					proxy.WithMethodEnforcement(http.MethodPost,
						proxy.WithAuth(cfg,
							proxy.WithRateLimit(rateLimiter,
								proxy.WithInlineGovernance(cfg, sigClient,
									proxy.WithShadowSignalAnalysis(cfg, sigClient, upstream),
								),
							),
						),
					),
				),
			),
		),
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	if metrics != nil {
		mux.Handle("/metrics", metrics.Handler())
	}
	mux.Handle("/v1/chat/completions", chatHandler)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
	}

	go func() {
		logger.Info("server_listening", "metrics", cfg.MetricsEnabled, "mode_label", modeLabel)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server_failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Info("shutting_down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hsar-org/hsar/internal/config"
	"github.com/hsar-org/hsar/internal/engine"
	"github.com/hsar-org/hsar/internal/policy"
	"github.com/hsar-org/hsar/internal/proxy"
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

	pol, err := policy.Load("")
	if err != nil {
		logger.Error("policy_load_failed", "error", err)
		os.Exit(1)
	}
	evaluator := policy.NewEvaluator(pol)
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

	chatHandler := proxy.WithLogging(
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
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.Handle("/v1/chat/completions", chatHandler)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
	}

	go func() {
		logger.Info("server_listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server_failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Info("shutting_down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}
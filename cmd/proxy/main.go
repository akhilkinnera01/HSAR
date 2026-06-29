package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hsar-org/hsar/internal/engine"
	"github.com/hsar-org/hsar/internal/proxy"
)

type Config struct {
	Port       string
	BackendURL string
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := loadConfig()
	logger.Info("starting_hsar_proxy",
		"port", cfg.Port,
		"backend", cfg.BackendURL,
	)

	var sigClient *engine.Client
	sigClient, err := engine.NewClientFromEnv()
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

	backendURL, err := url.Parse(cfg.BackendURL)
	if err != nil {
		logger.Error("invalid_backend_url", "error", err)
		os.Exit(1)
	}

	rp := httputil.NewSingleHostReverseProxy(backendURL)

	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Del("Connection")
		req.Header.Del("Keep-Alive")
		req.Header.Del("Proxy-Authenticate")
		req.Header.Del("Proxy-Authorization")
		req.Header.Del("Te")
		req.Header.Del("Trailers")
		req.Header.Del("Transfer-Encoding")
		req.Header.Del("Upgrade")
	}

	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error("backend_connection_failed",
			"trace_id", r.Header.Get("X-Request-ID"),
			"error", err,
		)
		http.Error(w, "Bad Gateway: Backend Unavailable", http.StatusBadGateway)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.Handle("/v1/chat/completions",
		proxy.WithLogging(
			proxy.WithTraceID(
				proxy.WithRequestDeadline(15*time.Second,
					proxy.WithMethodEnforcement(http.MethodPost,
						proxy.WithShadowSignalAnalysis(sigClient, rp),
					),
				),
			),
		),
	)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
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

func loadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	backend := os.Getenv("BACKEND_URL")
	if backend == "" {
		backend = "http://localhost:8081"
	}
	return Config{
		Port:       port,
		BackendURL: backend,
	}
}
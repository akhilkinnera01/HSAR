package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/hsar-org/hsar/internal/engine"
)

// Config holds the runtime configuration
type Config struct {
	Port       string
	BackendURL string
}

func main() {
	// 1. Setup Structured Logging (JSON)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// 2. Load Configuration
	cfg := loadConfig()
	logger.Info("starting_hsar_proxy", "port", cfg.Port, "backend", cfg.BackendURL)

	// 3. Initialize Signal Engine Client (fail-open)
	var signalClient *engine.Client
	sc, err := engine.NewClientFromEnv()
	if err != nil {
		logger.Warn("signal_engine_unavailable", "error", err)
	} else {
		signalClient = sc
		logger.Info("signal_engine_connected", "target", os.Getenv("SIGNAL_ENGINE_TARGET"))
	}

	// 4. Setup Reverse Proxy (The Fail-Open Path)
	backendURL, err := url.Parse(cfg.BackendURL)
	if err != nil {
		logger.Error("invalid_backend_url", "error", err)
		os.Exit(1)
	}

	proxy := httputil.NewSingleHostReverseProxy(backendURL)

	// Hop-by-hop header cleanup
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Del("Connection")
		req.Header.Del("Keep-Alive")
		req.Header.Del("Proxy-Authenticate")
		req.Header.Del("Proxy-Authorization")
		req.Header.Del("Te")
		req.Header.Del("Trailer")
		req.Header.Del("Transfer-Encoding")
		req.Header.Del("Upgrade")
	}

	// Custom Error Handler for the Proxy (Fail-Open visibility)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error("backend_connection_failed",
			"trace_id", r.Header.Get("X-Request-ID"),
			"error", err,
		)
		http.Error(w, "Bad Gateway: Backend Unavailable", http.StatusBadGateway)
	}

	// 5. Setup Router & Middleware
	mux := http.NewServeMux()

	// Health Check Endpoint
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Main Proxy Handler with Middleware Chain
	mux.Handle("/v1/chat/completions",
		withLogging(
			withTraceID(
				withMethodEnforcement(http.MethodPost,
					withRequestDeadline(15*time.Second,
						withSignalEngine(signalClient, proxy),
					),
				),
			),
		),
	)

	// 6. Start Server
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Graceful Shutdown Logic
	go func() {
		logger.Info("server_listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server_failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for Interrupt
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Info("shutting_down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if signalClient != nil {
		signalClient.Close()
	}
	server.Shutdown(ctx)
}

// =====================
// Middleware
// =====================

// withSignalEngine fires a non-blocking gRPC call to the signal engine
// before forwarding the request. If the client is nil, it passes through (fail-open).
func withSignalEngine(client *engine.Client, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client != nil {
			body, err := io.ReadAll(r.Body)
			r.Body.Close()
			if err == nil {
				// Restore body for the reverse proxy
				r.Body = io.NopCloser(bytes.NewReader(body))
				r.ContentLength = int64(len(body))

				traceID := r.Header.Get("X-Request-ID")
				go client.ShadowGetSignals("default", traceID, string(body))
			} else {
				slog.Warn("signal_engine_body_read_failed", "error", err)
				// Restore empty body so the proxy doesn't break
				r.Body = io.NopCloser(bytes.NewReader(nil))
				r.ContentLength = 0
			}
		}
		next.ServeHTTP(w, r)
	})
}

// withMethodEnforcement ensures only specific HTTP methods are allowed
func withMethodEnforcement(allowedMethod string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != allowedMethod {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withRequestDeadline enforces a strict timeout for the entire request lifecycle
func withRequestDeadline(timeout time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// withTraceID ensures every request has a unique ID for observability
func withTraceID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := r.Header.Get("X-Request-ID")
		if traceID == "" {
			traceID = uuid.New().String()
			r.Header.Set("X-Request-ID", traceID)
		}
		w.Header().Set("X-Request-ID", traceID)
		next.ServeHTTP(w, r)
	})
}

// withLogging logs the request duration and status (Structured)
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWrapper{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(ww, r)
		slog.Info("request_served",
			"trace_id", r.Header.Get("X-Request-ID"),
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// =====================
// Helpers
// =====================

type responseWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
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

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

	"github.com/google/uuid"
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

	// 3. Setup Reverse Proxy (The Fail-Open Path)
	backendURL, err := url.Parse(cfg.BackendURL)
	if err != nil {
		logger.Error("invalid_backend_url", "error", err)
		os.Exit(1)
	}

	proxy := httputil.NewSingleHostReverseProxy(backendURL)

	// (Optional) Hop-by-hop header cleanup
	// NewSingleHostReverseProxy does a lot, but explicit cleanup is safer.
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Remove hop-by-hop headers to prevent connection issues upstream
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

	// 4. Setup Router & Middleware
	mux := http.NewServeMux()

	// Health Check Endpoint (Standard Ops)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Main Proxy Handler with Method Enforcement
	// We wrap the proxy with our "Intervention" middleware
	// For Step 2, this just logs and forwards (Pass-through)
	mux.Handle("/v1/chat/completions",
		withLogging(
			withTraceID(
				withMethodEnforcement(http.MethodPost,
					withRequestDeadline(15*time.Second, proxy), // Request-scoped deadline
				),
			),
		),
	)

	// 5. Start Server
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
	server.Shutdown(ctx)
}

// =====================
// Middleware
// =====================

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
		// Create a context with a hard deadline
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		// Pass the new context down the chain
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
		// Pass it back to the client too
		w.Header().Set("X-Request-ID", traceID)
		next.ServeHTTP(w, r)
	})
}

// withLogging logs the request duration and status (Structured)
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Use a wrapper to capture the status code
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
		// Default to a local mock if not set
		backend = "http://localhost:8081"
	}

	return Config{
		Port:       port,
		BackendURL: backend,
	}
}

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

	// Import our internal engine client
	"github.com/hsar-org/hsar/internal/engine"
)

type Config struct {
	Port       string
	BackendURL string
}

func main() {
	// 1. Setup Structured Logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := loadConfig()
	logger.Info("starting_hsar_proxy",
		"port", cfg.Port,
		"backend", cfg.BackendURL,
	)

	// 2. Initialize Signal Engine Client
	// Modified logic per request: Log warning on failure, set client to nil, proceed fail-open.
	var sigClient *engine.Client
	var err error

	sigClient, err = engine.NewClientFromEnv()
	if err != nil {
		slog.Warn("signal_engine_disabled", "error", err)
		sigClient = nil
	} else {
		logger.Info("signal_engine_connected")
	}

	// Ensure cleanup happens if client was successfully created
	defer func() {
		if sigClient != nil {
			_ = sigClient.Close()
		}
	}()

	// 3. Setup Reverse Proxy
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
		req.Header.Del("Trailers")
		req.Header.Del("Transfer-Encoding")
		req.Header.Del("Upgrade")
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error("backend_connection_failed",
			"trace_id", r.Header.Get("X-Request-ID"),
			"error", err,
		)
		http.Error(w, "Bad Gateway: Backend Unavailable", http.StatusBadGateway)
	}

	// 4. Setup Router & Middleware Chain
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// The Steel Thread Chain with Shadow Call
	mux.Handle("/v1/chat/completions",
		withLogging(
			withTraceID(
				withRequestDeadline(15*time.Second,
					withMethodEnforcement(http.MethodPost,
						// Inject the Shadow Call middleware here
						withShadowSignalAnalysis(sigClient, proxy),
					),
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

// =====================
// Middleware
// =====================

// withShadowSignalAnalysis forks a goroutine to call the signal engine.
// It NEVER blocks the main request flow.
func withShadowSignalAnalysis(client *engine.Client, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If client failed to init, skip everything (Fail Open)
		if client == nil {
			next.ServeHTTP(w, r)
			return
		}

		// 1. Read body to get text (efficiently)
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			slog.Error("failed_to_read_body", "error", err)
			next.ServeHTTP(w, r)
			return
		}
		// Restore body for proxy
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// 2. Launch Shadow Call (Fire and Forget)
		reqID := r.Header.Get("X-Request-ID")
		// Use a local copy of data to avoid race conditions if bodyBytes is reused (it's safe here)
		textPayload := string(bodyBytes) // For MVP Step 3, treat JSON as text string

		go func() {
			// This runs in background. Does not delay response.
			client.ShadowGetSignals("default-tenant", reqID, textPayload)
		}()

		// 3. Proceed immediately
		next.ServeHTTP(w, r)
	})
}

func withMethodEnforcement(allowedMethod string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != allowedMethod {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withRequestDeadline(timeout time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

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

package proxy

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

func WithMethodEnforcement(allowedMethod string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != allowedMethod {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func WithRequestDeadline(timeout time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func WithTraceID(next http.Handler) http.Handler {
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

func WithLogging(next http.Handler) http.Handler {
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
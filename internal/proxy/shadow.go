package proxy

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
)

// SignalAnalyzer performs non-blocking shadow signal extraction.
type SignalAnalyzer interface {
	ShadowGetSignals(tenantID, requestID, text string)
}

func WithShadowSignalAnalysis(client SignalAnalyzer, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			next.ServeHTTP(w, r)
			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			slog.Error("failed_to_read_body", "error", err)
			next.ServeHTTP(w, r)
			return
		}
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		reqID := r.Header.Get("X-Request-ID")
		textPayload := string(bodyBytes)
		tenantID := "default-tenant"
		if tid, ok := TenantFromContext(r.Context()); ok {
			tenantID = tid
		}

		go client.ShadowGetSignals(tenantID, reqID, textPayload)

		next.ServeHTTP(w, r)
	})
}
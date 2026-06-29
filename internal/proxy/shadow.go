package proxy

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"

	"github.com/hsar-org/hsar/internal/config"
)

// SignalAnalyzer performs non-blocking shadow signal extraction.
type SignalAnalyzer interface {
	ShadowGetSignals(tenantID, requestID, conversationID, text string)
}

func WithShadowSignalAnalysis(cfg config.Config, client SignalAnalyzer, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			slog.Error("failed_to_read_body", "error", err)
			next.ServeHTTP(w, r)
			return
		}
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		reqID := r.Header.Get("X-Request-ID")
		if client != nil && ShouldRunShadowAsync(cfg, reqID) {
			conversationID := r.Header.Get("X-Conversation-ID")
			if conversationID == "" {
				conversationID = reqID
			}
			textPayload := string(bodyBytes)
			tenantID := "default-tenant"
			if tid, ok := TenantFromContext(r.Context()); ok {
				tenantID = tid
			}
			go client.ShadowGetSignals(tenantID, reqID, conversationID, textPayload)
		}

		next.ServeHTTP(w, r)
	})
}
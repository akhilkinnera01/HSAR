package proxy_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hsar-org/hsar/internal/proxy"
)

func TestWithMethodEnforcementRejectsNonPOST(t *testing.T) {
	t.Parallel()

	ok := httptest.NewRecorder()
	proxy.WithMethodEnforcement(http.MethodPost, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(ok, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if ok.Code != http.StatusOK {
		t.Fatalf("POST: got status %d, want %d", ok.Code, http.StatusOK)
	}

	bad := httptest.NewRecorder()
	proxy.WithMethodEnforcement(http.MethodPost, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil))
	if bad.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: got status %d, want %d", bad.Code, http.StatusMethodNotAllowed)
	}
}

func TestWithTraceIDPropagatesOrGenerates(t *testing.T) {
	t.Parallel()

	t.Run("generates when missing", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", nil)

		proxy.WithTraceID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("X-Request-ID"); got == "" {
				t.Fatal("expected request trace ID to be set")
			}
		})).ServeHTTP(rec, req)

		if got := rec.Header().Get("X-Request-ID"); got == "" {
			t.Fatal("expected response trace ID header")
		}
	})

	t.Run("preserves incoming", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("X-Request-ID", "trace-abc")

		proxy.WithTraceID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("X-Request-ID"); got != "trace-abc" {
				t.Fatalf("request trace ID = %q, want trace-abc", got)
			}
		})).ServeHTTP(rec, req)

		if got := rec.Header().Get("X-Request-ID"); got != "trace-abc" {
			t.Fatalf("response trace ID = %q, want trace-abc", got)
		}
	})
}
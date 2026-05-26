package proxy_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hsar-org/hsar/internal/config"
	"github.com/hsar-org/hsar/internal/proxy"
)

func testConfig() config.Config {
	return config.Config{
		TenantsByKey: map[string]config.Tenant{
			"good-key": {ID: "tenant-1", APIKey: "good-key"},
		},
	}
}

func TestWithAuthRejectsMissingKey(t *testing.T) {
	rec := httptest.NewRecorder()
	proxy.WithAuth(testConfig(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestWithAuthRejectsInvalidKey(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer bad-key")
	proxy.WithAuth(testConfig(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestWithAuthAcceptsBearerAndSetsTenant(t *testing.T) {
	var gotTenant string
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer good-key")
	proxy.WithAuth(testConfig(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenant, _ = proxy.TenantFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if gotTenant != "tenant-1" {
		t.Fatalf("tenant = %q", gotTenant)
	}
}
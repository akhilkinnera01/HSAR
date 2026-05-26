package proxy_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hsar-org/hsar/internal/config"
	"github.com/hsar-org/hsar/internal/proxy"
)

func TestWithRateLimitReturns429(t *testing.T) {
	cfg := config.Config{
		DefaultRPS:   1,
		DefaultBurst: 1,
		TenantsByKey: map[string]config.Tenant{
			"k": {ID: "t1", APIKey: "k", RateLimitRPS: 1, RateBurst: 1},
		},
	}
	rl := proxy.NewRateLimiter(cfg)
	handler := proxy.WithRateLimit(rl, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	chain := proxy.WithAuth(cfg, handler)

	do := func() int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("X-API-Key", "k")
		chain.ServeHTTP(rec, req)
		return rec.Code
	}

	if do() != http.StatusOK {
		t.Fatal("first request should succeed")
	}
	if do() != http.StatusTooManyRequests {
		t.Fatal("second request should be rate limited")
	}
}
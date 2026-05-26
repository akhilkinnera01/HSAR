package proxy

import (
	"net/http"
	"sync"
	"time"

	"github.com/hsar-org/hsar/internal/config"
)

type bucket struct {
	tokens     float64
	lastRefill time.Time
	rps        float64
	burst      float64
}

type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	cfg     config.Config
}

func NewRateLimiter(cfg config.Config) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*bucket),
		cfg:     cfg,
	}
}

func (rl *RateLimiter) allow(tenantID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rps, burst := rl.cfg.DefaultRPS, float64(rl.cfg.DefaultBurst)
	for _, t := range rl.cfg.TenantsByKey {
		if t.ID == tenantID {
			rps = t.RateLimitRPS
			burst = float64(t.RateBurst)
			break
		}
	}

	b, ok := rl.buckets[tenantID]
	if !ok {
		b = &bucket{tokens: burst, lastRefill: time.Now(), rps: rps, burst: burst}
		rl.buckets[tenantID] = b
	}

	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens = minFloat(b.burst, b.tokens+elapsed*b.rps)
	b.lastRefill = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func WithRateLimit(rl *RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := TenantFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if !rl.allow(tenantID) {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
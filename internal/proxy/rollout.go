package proxy

import (
	"hash/fnv"
	"os"
	"strconv"

	"github.com/hsar-org/hsar/internal/config"
)

// IsKillSwitchActive reads ENFORCE_KILL_SWITCH at request time for instant effect.
func IsKillSwitchActive(cfg config.Config) bool {
	if v := os.Getenv("ENFORCE_KILL_SWITCH"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return cfg.EnforceKillSwitch
}

// InCanaryCohort returns true when requestID falls in the canary percentage bucket.
func InCanaryCohort(requestID string, canaryPct int) bool {
	if canaryPct <= 0 {
		return false
	}
	if canaryPct >= 100 {
		return true
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(requestID))
	return int(h.Sum32()%100) < canaryPct
}

// ShouldEnforce reports whether inline governance may mutate this request.
func ShouldEnforce(cfg config.Config, requestID string) bool {
	if IsKillSwitchActive(cfg) {
		return false
	}
	switch cfg.Mode {
	case config.ModeEnforce:
		return true
	case config.ModeCanary:
		return InCanaryCohort(requestID, cfg.CanaryPct)
	default:
		return false
	}
}

// ShouldRunShadowAsync reports whether the async shadow goroutine should run.
func ShouldRunShadowAsync(cfg config.Config, requestID string) bool {
	if IsKillSwitchActive(cfg) {
		return true
	}
	switch cfg.Mode {
	case config.ModeShadow:
		return true
	case config.ModeCanary:
		return !InCanaryCohort(requestID, cfg.CanaryPct)
	default:
		return false
	}
}
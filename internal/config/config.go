package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Mode string

const (
	ModeShadow  Mode = "shadow"
	ModeCanary  Mode = "canary"
	ModeEnforce Mode = "enforce"
)

type Tenant struct {
	ID           string  `yaml:"id"`
	APIKey       string  `yaml:"api_key"`
	RateLimitRPS float64 `yaml:"rate_limit_rps"`
	RateBurst    int     `yaml:"rate_burst"`
}

type Config struct {
	Port            string
	UpstreamBaseURL string
	Mode            Mode
	CanaryPct       int
	TenantsByKey    map[string]Tenant
	DefaultRPS      float64
	DefaultBurst    int
}

type fileConfig struct {
	UpstreamBaseURL string   `yaml:"upstream_base_url"`
	Mode            string   `yaml:"mode"`
	CanaryPct       int      `yaml:"canary_pct"`
	Tenants         []Tenant `yaml:"tenants"`
}

func Load() (Config, error) {
	cfg := Config{
		Port:            envOr("PORT", "8080"),
		UpstreamBaseURL: envOr("UPSTREAM_BASE_URL", envOr("BACKEND_URL", "http://localhost:8081")),
		Mode:            Mode(envOr("MODE", string(ModeShadow))),
		CanaryPct:       envIntOr("CANARY_PCT", 0),
		DefaultRPS:      envFloatOr("TENANT_RATE_RPS", 10),
		DefaultBurst:    envIntOr("TENANT_RATE_BURST", 20),
		TenantsByKey:    map[string]Tenant{},
	}

	if path := os.Getenv("CONFIG_PATH"); path != "" {
		if err := overlayFile(&cfg, path); err != nil {
			return Config{}, err
		}
	}

	if keys := os.Getenv("TENANT_API_KEYS"); keys != "" {
		parseTenantKeys(&cfg, keys)
	}

	if len(cfg.TenantsByKey) == 0 {
		cfg.TenantsByKey["dev-key-1"] = Tenant{
			ID:           "default-tenant",
			APIKey:       "dev-key-1",
			RateLimitRPS: cfg.DefaultRPS,
			RateBurst:    cfg.DefaultBurst,
		}
	}

	for key, t := range cfg.TenantsByKey {
		if t.RateLimitRPS <= 0 {
			t.RateLimitRPS = cfg.DefaultRPS
		}
		if t.RateBurst <= 0 {
			t.RateBurst = cfg.DefaultBurst
		}
		if t.ID == "" {
			t.ID = "tenant-" + key[:min(8, len(key))]
		}
		t.APIKey = key
		cfg.TenantsByKey[key] = t
	}

	switch cfg.Mode {
	case ModeShadow, ModeCanary, ModeEnforce:
	default:
		return Config{}, fmt.Errorf("invalid MODE %q", cfg.Mode)
	}

	if cfg.UpstreamBaseURL == "" {
		return Config{}, fmt.Errorf("UPSTREAM_BASE_URL is required")
	}

	return cfg, nil
}

func overlayFile(cfg *Config, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	var fc fileConfig
	if err := yaml.Unmarshal(raw, &fc); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if fc.UpstreamBaseURL != "" {
		cfg.UpstreamBaseURL = fc.UpstreamBaseURL
	}
	if fc.Mode != "" {
		cfg.Mode = Mode(fc.Mode)
	}
	if fc.CanaryPct > 0 {
		cfg.CanaryPct = fc.CanaryPct
	}
	for _, t := range fc.Tenants {
		if t.APIKey == "" {
			continue
		}
		cfg.TenantsByKey[t.APIKey] = t
	}
	return nil
}

func parseTenantKeys(cfg *Config, keys string) {
	for _, pair := range strings.Split(keys, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key, id := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		cfg.TenantsByKey[key] = Tenant{ID: id, APIKey: key}
	}
}

func (c Config) LookupTenant(apiKey string) (Tenant, bool) {
	t, ok := c.TenantsByKey[apiKey]
	return t, ok
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envIntOr(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envFloatOr(k string, def float64) float64 {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return n
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
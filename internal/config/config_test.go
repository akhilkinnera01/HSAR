package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hsar-org/hsar/internal/config"
)

func TestLoadDefaultsDevTenant(t *testing.T) {
	t.Setenv("UPSTREAM_BASE_URL", "http://upstream:8081")
	t.Setenv("TENANT_API_KEYS", "")
	t.Setenv("CONFIG_PATH", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.UpstreamBaseURL != "http://upstream:8081" {
		t.Fatalf("upstream = %q", cfg.UpstreamBaseURL)
	}
	if _, ok := cfg.TenantsByKey["dev-key-1"]; !ok {
		t.Fatal("expected default dev-key-1 tenant")
	}
}

func TestLoadTenantAPIKeysEnv(t *testing.T) {
	t.Setenv("TENANT_API_KEYS", "key-a:tenant-a,key-b:tenant-b")
	t.Setenv("CONFIG_PATH", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ta, ok := cfg.LookupTenant("key-a")
	if !ok || ta.ID != "tenant-a" {
		t.Fatalf("tenant-a: %+v ok=%v", ta, ok)
	}
}

func TestLoadYAMLOverlay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(`
upstream_base_url: http://yaml-upstream:9999
mode: shadow
tenants:
  - id: yaml-tenant
    api_key: yaml-key
    rate_limit_rps: 5
    rate_burst: 10
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFIG_PATH", path)
	t.Setenv("TENANT_API_KEYS", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.UpstreamBaseURL != "http://yaml-upstream:9999" {
		t.Fatalf("upstream = %q", cfg.UpstreamBaseURL)
	}
	yt, ok := cfg.LookupTenant("yaml-key")
	if !ok || yt.ID != "yaml-tenant" {
		t.Fatalf("yaml tenant: %+v", yt)
	}
}
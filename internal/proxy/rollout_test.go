package proxy_test

import (
	"fmt"
	"testing"

	"github.com/hsar-org/hsar/internal/config"
	"github.com/hsar-org/hsar/internal/proxy"
)

func TestInCanaryCohortDeterministic(t *testing.T) {
	t.Parallel()

	id := "req-canary-deterministic"
	a := proxy.InCanaryCohort(id, 50)
	b := proxy.InCanaryCohort(id, 50)
	if a != b {
		t.Fatal("cohort decision not deterministic")
	}
}

func TestCanaryDistributionApprox10Pct(t *testing.T) {
	t.Parallel()

	enforce := 0
	const n = 1000
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("req-canary-%d", i)
		if proxy.InCanaryCohort(id, 10) {
			enforce++
		}
	}
	pct := float64(enforce) / float64(n) * 100
	if pct < 5 || pct > 15 {
		t.Fatalf("canary pct = %.1f, want ~10", pct)
	}
}

func TestShouldEnforceModes(t *testing.T) {
	t.Parallel()

	shadowCfg := config.Config{Mode: config.ModeShadow}
	if proxy.ShouldEnforce(shadowCfg, "req-1") {
		t.Fatal("shadow should not enforce")
	}

	enforceCfg := config.Config{Mode: config.ModeEnforce}
	if !proxy.ShouldEnforce(enforceCfg, "req-1") {
		t.Fatal("enforce mode should enforce")
	}

	canaryCfg := config.Config{Mode: config.ModeCanary, CanaryPct: 100}
	if !proxy.ShouldEnforce(canaryCfg, "req-1") {
		t.Fatal("canary 100% should enforce")
	}
}

func TestKillSwitchDisablesEnforce(t *testing.T) {
	t.Parallel()

	cfg := config.Config{Mode: config.ModeEnforce, EnforceKillSwitch: true}
	if proxy.ShouldEnforce(cfg, "req-1") {
		t.Fatal("kill switch should disable enforce")
	}
}

func TestKillSwitchEnvOverridesConfig(t *testing.T) {
	t.Setenv("ENFORCE_KILL_SWITCH", "true")
	cfg := config.Config{Mode: config.ModeEnforce, EnforceKillSwitch: false}
	if proxy.ShouldEnforce(cfg, "req-1") {
		t.Fatal("env kill switch should disable enforce")
	}
}
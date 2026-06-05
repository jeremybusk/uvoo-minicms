package config

import (
	"flag"
	"os"
	"testing"
)

func TestLoadReadsAdminRateLimitFromEnv(t *testing.T) {
	t.Setenv("CMS_ADMIN_RATE_LIMIT", "120")
	cfg := loadForTest(t)
	if cfg.AdminRateLimit != 120 {
		t.Fatalf("expected admin rate limit 120, got %d", cfg.AdminRateLimit)
	}
}

func TestLoadTreatsInvalidAdminRateLimitAsDisabled(t *testing.T) {
	t.Setenv("CMS_ADMIN_RATE_LIMIT", "-25")
	cfg := loadForTest(t)
	if cfg.AdminRateLimit != 0 {
		t.Fatalf("expected invalid negative rate limit to disable, got %d", cfg.AdminRateLimit)
	}
}

func TestLoadReadsAdminRateLimitFlag(t *testing.T) {
	cfg := loadForTest(t, "-admin-rate-limit", "45")
	if cfg.AdminRateLimit != 45 {
		t.Fatalf("expected admin rate limit 45, got %d", cfg.AdminRateLimit)
	}
}

func loadForTest(t *testing.T, args ...string) Config {
	t.Helper()
	oldCommandLine := flag.CommandLine
	oldArgs := os.Args
	t.Cleanup(func() {
		flag.CommandLine = oldCommandLine
		os.Args = oldArgs
	})
	flag.CommandLine = flag.NewFlagSet("config-test", flag.ContinueOnError)
	os.Args = append([]string{"uvoo-minicms"}, args...)
	return Load()
}

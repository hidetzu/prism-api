package config

import (
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := map[string]any{
		"Port":                  "8080",
		"LogLevel":              "info",
		"RequestTimeout":        30 * time.Second,
		"ShutdownTimeout":       25 * time.Second,
		"MaxRequestBytes":       int64(256 * 1024),
		"RateLimitRPM":          10,
		"RateLimitBurst":        20,
		"MaxConcurrentRequests": 50,
		"MaxChangedFiles":       50,
		"MaxDiffBytes":          int64(200 * 1024),
		"MaxResponseBytes":      int64(500 * 1024),
	}

	got := map[string]any{
		"Port":                  cfg.Port,
		"LogLevel":              cfg.LogLevel,
		"RequestTimeout":        cfg.RequestTimeout,
		"ShutdownTimeout":       cfg.ShutdownTimeout,
		"MaxRequestBytes":       cfg.MaxRequestBytes,
		"RateLimitRPM":          cfg.RateLimitRPM,
		"RateLimitBurst":        cfg.RateLimitBurst,
		"MaxConcurrentRequests": cfg.MaxConcurrentRequests,
		"MaxChangedFiles":       cfg.MaxChangedFiles,
		"MaxDiffBytes":          cfg.MaxDiffBytes,
		"MaxResponseBytes":      cfg.MaxResponseBytes,
	}

	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}

	if len(cfg.AllowedProviders) != 1 || cfg.AllowedProviders[0] != "github" {
		t.Errorf("AllowedProviders = %v, want [github]", cfg.AllowedProviders)
	}
}

func TestLoad_OverrideFromEnv(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("REQUEST_TIMEOUT", "10s")
	t.Setenv("MAX_CONCURRENT_REQUESTS", "100")
	t.Setenv("MAX_DIFF_BYTES", "1024")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want 9090", cfg.Port)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.RequestTimeout != 10*time.Second {
		t.Errorf("RequestTimeout = %v, want 10s", cfg.RequestTimeout)
	}
	if cfg.MaxConcurrentRequests != 100 {
		t.Errorf("MaxConcurrentRequests = %d, want 100", cfg.MaxConcurrentRequests)
	}
	if cfg.MaxDiffBytes != 1024 {
		t.Errorf("MaxDiffBytes = %d, want 1024", cfg.MaxDiffBytes)
	}
}

func TestLoad_InvalidDurationReturnsError(t *testing.T) {
	t.Setenv("REQUEST_TIMEOUT", "not-a-duration")
	if _, err := Load(); err == nil {
		t.Fatal("Load() should return error for malformed REQUEST_TIMEOUT")
	}
}

func TestLoad_InvalidIntReturnsError(t *testing.T) {
	t.Setenv("RATE_LIMIT_RPM", "not-a-number")
	if _, err := Load(); err == nil {
		t.Fatal("Load() should return error for malformed RATE_LIMIT_RPM")
	}
}

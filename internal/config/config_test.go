package config

import (
	"strings"
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

func TestLoad_AllowedProvidersFromEnv(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want []string
	}{
		{"single", "gitlab", []string{"gitlab"}},
		{"multiple", "github,gitlab", []string{"github", "gitlab"}},
		{"trims whitespace", " github , gitlab ", []string{"github", "gitlab"}},
		{"ignores empty entries", "github,,gitlab,", []string{"github", "gitlab"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ALLOWED_PROVIDERS", tc.env)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if len(cfg.AllowedProviders) != len(tc.want) {
				t.Fatalf("AllowedProviders = %v, want %v", cfg.AllowedProviders, tc.want)
			}
			for i, v := range tc.want {
				if cfg.AllowedProviders[i] != v {
					t.Errorf("AllowedProviders[%d] = %q, want %q", i, cfg.AllowedProviders[i], v)
				}
			}
		})
	}
}

func TestLoad_AllowedProvidersFallsBackOnBlankEnv(t *testing.T) {
	t.Setenv("ALLOWED_PROVIDERS", " , ,")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.AllowedProviders) != 1 || cfg.AllowedProviders[0] != "github" {
		t.Errorf("AllowedProviders = %v, want [github] fallback", cfg.AllowedProviders)
	}
}

func TestLoad_RejectsOutOfRangeValues(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		wantSub string // substring expected in error message
	}{
		{
			name:    "zero request timeout",
			env:     map[string]string{"REQUEST_TIMEOUT": "0s"},
			wantSub: "REQUEST_TIMEOUT",
		},
		{
			name:    "negative request timeout",
			env:     map[string]string{"REQUEST_TIMEOUT": "-1s"},
			wantSub: "REQUEST_TIMEOUT",
		},
		{
			name:    "zero shutdown timeout",
			env:     map[string]string{"SHUTDOWN_TIMEOUT": "0s"},
			wantSub: "SHUTDOWN_TIMEOUT",
		},
		{
			name:    "negative shutdown timeout",
			env:     map[string]string{"SHUTDOWN_TIMEOUT": "-5s"},
			wantSub: "SHUTDOWN_TIMEOUT",
		},
		{
			name:    "negative max request bytes",
			env:     map[string]string{"MAX_REQUEST_BYTES": "-1"},
			wantSub: "MAX_REQUEST_BYTES",
		},
		{
			name:    "negative rate limit rpm",
			env:     map[string]string{"RATE_LIMIT_RPM": "-10"},
			wantSub: "RATE_LIMIT_RPM",
		},
		{
			name:    "negative rate limit burst",
			env:     map[string]string{"RATE_LIMIT_BURST": "-1"},
			wantSub: "RATE_LIMIT_BURST",
		},
		{
			name:    "negative max concurrent",
			env:     map[string]string{"MAX_CONCURRENT_REQUESTS": "-5"},
			wantSub: "MAX_CONCURRENT_REQUESTS",
		},
		{
			name:    "negative max changed files",
			env:     map[string]string{"MAX_CHANGED_FILES": "-1"},
			wantSub: "MAX_CHANGED_FILES",
		},
		{
			name:    "negative max diff bytes",
			env:     map[string]string{"MAX_DIFF_BYTES": "-100"},
			wantSub: "MAX_DIFF_BYTES",
		},
		{
			name:    "negative max response bytes",
			env:     map[string]string{"MAX_RESPONSE_BYTES": "-100"},
			wantSub: "MAX_RESPONSE_BYTES",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			_, err := Load()
			if err == nil {
				t.Fatal("Load() returned nil error, want validation error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestLoad_ZeroIsValidForCountFields(t *testing.T) {
	// Zero is accepted for count/byte fields because the middleware layer
	// uses it as the "disable this guard" convention.
	t.Setenv("MAX_REQUEST_BYTES", "0")
	t.Setenv("RATE_LIMIT_RPM", "0")
	t.Setenv("RATE_LIMIT_BURST", "0")
	t.Setenv("MAX_CONCURRENT_REQUESTS", "0")
	t.Setenv("MAX_CHANGED_FILES", "0")
	t.Setenv("MAX_DIFF_BYTES", "0")
	t.Setenv("MAX_RESPONSE_BYTES", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err = %v, want nil (zero is valid for count fields)", err)
	}
	if cfg.MaxRequestBytes != 0 || cfg.RateLimitRPM != 0 || cfg.MaxConcurrentRequests != 0 {
		t.Errorf("zero count fields not preserved: %+v", cfg)
	}
}

func TestConfig_Validate_ValidDefault(t *testing.T) {
	// Sanity check: the default-loaded Config must pass Validate so that
	// TestLoad_Defaults remains meaningful.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err = %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("default Validate() err = %v, want nil", err)
	}
}

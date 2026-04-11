// Package config loads runtime configuration from environment variables.
//
// All configuration is read once at startup. Missing variables fall back to
// documented defaults; malformed values cause Load to return an error so that
// misconfigurations fail loudly instead of silently.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime configuration for prism-api.
type Config struct {
	Port            string
	LogLevel        string
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration

	// Reserved for /v1/analyze and /v1/prompt endpoints. Loaded at startup so
	// the defense middleware can be wired in the next phase without revisiting
	// the loader.
	MaxRequestBytes       int64
	RateLimitRPM          int
	RateLimitBurst        int
	MaxConcurrentRequests int
	MaxChangedFiles       int
	MaxDiffBytes          int64
	MaxResponseBytes      int64
	AllowedProviders      []string
}

// Load reads Config from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		Port:             getString("PORT", "8080"),
		LogLevel:         getString("LOG_LEVEL", "info"),
		AllowedProviders: getStringSlice("ALLOWED_PROVIDERS", []string{"github"}),
	}

	var err error
	if cfg.RequestTimeout, err = getDuration("REQUEST_TIMEOUT", 30*time.Second); err != nil {
		return nil, err
	}
	if cfg.ShutdownTimeout, err = getDuration("SHUTDOWN_TIMEOUT", 25*time.Second); err != nil {
		return nil, err
	}
	if cfg.MaxRequestBytes, err = getInt64("MAX_REQUEST_BYTES", 256*1024); err != nil {
		return nil, err
	}
	if cfg.RateLimitRPM, err = getInt("RATE_LIMIT_RPM", 10); err != nil {
		return nil, err
	}
	if cfg.RateLimitBurst, err = getInt("RATE_LIMIT_BURST", 20); err != nil {
		return nil, err
	}
	if cfg.MaxConcurrentRequests, err = getInt("MAX_CONCURRENT_REQUESTS", 50); err != nil {
		return nil, err
	}
	if cfg.MaxChangedFiles, err = getInt("MAX_CHANGED_FILES", 50); err != nil {
		return nil, err
	}
	if cfg.MaxDiffBytes, err = getInt64("MAX_DIFF_BYTES", 200*1024); err != nil {
		return nil, err
	}
	if cfg.MaxResponseBytes, err = getInt64("MAX_RESPONSE_BYTES", 500*1024); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// validLogLevels mirrors the set accepted by logging.parseLevel. It is
// duplicated here (rather than imported from internal/logging) because
// logging depends on config and importing it back would create a cycle.
// The set is tiny and stable, so the duplication cost is minimal.
var validLogLevels = map[string]struct{}{
	"debug":   {},
	"info":    {},
	"warn":    {},
	"warning": {},
	"error":   {},
}

// Validate checks that loaded field values are within acceptable bounds.
//
// Rules:
//   - PORT must parse as an integer in [0, 65535]. Zero is accepted so that
//     tests and ephemeral-port use cases still work.
//   - LOG_LEVEL must match one of the slog level names parseLevel accepts
//     (debug, info, warn, warning, error), case-insensitive.
//   - Duration fields must be strictly positive because a zero or negative
//     timeout is never a useful configuration.
//   - Count and byte fields must be non-negative; a value of zero is
//     accepted because body_limit / rate_limit / concurrency_limit
//     middleware document it as the "disable this guard" convention.
//
// All violations are collected and returned together via errors.Join so
// an operator fixing a misconfiguration can see every problem at once
// rather than one per restart.
func (c *Config) Validate() error {
	var errs []error

	if p, err := strconv.Atoi(c.Port); err != nil || p < 0 || p > 65535 {
		errs = append(errs, fmt.Errorf("PORT must be an integer in [0, 65535], got %q", c.Port))
	}
	if _, ok := validLogLevels[strings.ToLower(c.LogLevel)]; !ok {
		errs = append(errs, fmt.Errorf("LOG_LEVEL must be one of debug|info|warn|warning|error, got %q", c.LogLevel))
	}
	if c.RequestTimeout <= 0 {
		errs = append(errs, fmt.Errorf("REQUEST_TIMEOUT must be positive, got %v", c.RequestTimeout))
	}
	if c.ShutdownTimeout <= 0 {
		errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT must be positive, got %v", c.ShutdownTimeout))
	}
	if c.MaxRequestBytes < 0 {
		errs = append(errs, fmt.Errorf("MAX_REQUEST_BYTES must be non-negative, got %d", c.MaxRequestBytes))
	}
	if c.RateLimitRPM < 0 {
		errs = append(errs, fmt.Errorf("RATE_LIMIT_RPM must be non-negative, got %d", c.RateLimitRPM))
	}
	if c.RateLimitBurst < 0 {
		errs = append(errs, fmt.Errorf("RATE_LIMIT_BURST must be non-negative, got %d", c.RateLimitBurst))
	}
	if c.MaxConcurrentRequests < 0 {
		errs = append(errs, fmt.Errorf("MAX_CONCURRENT_REQUESTS must be non-negative, got %d", c.MaxConcurrentRequests))
	}
	if c.MaxChangedFiles < 0 {
		errs = append(errs, fmt.Errorf("MAX_CHANGED_FILES must be non-negative, got %d", c.MaxChangedFiles))
	}
	if c.MaxDiffBytes < 0 {
		errs = append(errs, fmt.Errorf("MAX_DIFF_BYTES must be non-negative, got %d", c.MaxDiffBytes))
	}
	if c.MaxResponseBytes < 0 {
		errs = append(errs, fmt.Errorf("MAX_RESPONSE_BYTES must be non-negative, got %d", c.MaxResponseBytes))
	}

	return errors.Join(errs...)
}

func getString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// getStringSlice reads a comma-separated environment variable and returns
// the non-empty trimmed entries. If the variable is unset or yields no
// non-empty entries, def is returned.
func getStringSlice(key string, def []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

func getInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return n, nil
}

func getInt64(key string, def int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return n, nil
}

func getDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return d, nil
}

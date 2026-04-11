// Package config loads runtime configuration from environment variables.
//
// All configuration is read once at startup. Missing variables fall back to
// documented defaults; malformed values cause Load to return an error so that
// misconfigurations fail loudly instead of silently.
package config

import (
	"fmt"
	"os"
	"strconv"
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
		AllowedProviders: []string{"github"},
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

	return cfg, nil
}

func getString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
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

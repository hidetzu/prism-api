package logging

import (
	"context"
	"log/slog"
	"testing"

	"github.com/hidetzu/prism-api/internal/config"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  slog.Level
	}{
		{"debug", "debug", slog.LevelDebug},
		{"debug uppercase", "DEBUG", slog.LevelDebug},
		{"info", "info", slog.LevelInfo},
		{"info mixed case", "Info", slog.LevelInfo},
		{"warn", "warn", slog.LevelWarn},
		{"warning alias", "warning", slog.LevelWarn},
		{"warning uppercase", "WARNING", slog.LevelWarn},
		{"error", "error", slog.LevelError},
		// Unknown input returns LevelInfo as defense in depth. In
		// production, config.Validate rejects unknown log levels before
		// this function is called, so this branch is unreachable via the
		// normal startup path. Keep the test anyway so a future refactor
		// that bypasses Load() still gets a sane default instead of a
		// panic or a bogus level.
		{"unknown falls back to info", "bogus", slog.LevelInfo},
		{"empty string falls back to info", "", slog.LevelInfo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseLevel(tc.input); got != tc.want {
				t.Errorf("parseLevel(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestNew_RespectsConfiguredLevel(t *testing.T) {
	cases := []struct {
		level     string
		wantDebug bool
		wantInfo  bool
		wantWarn  bool
		wantError bool
	}{
		{"debug", true, true, true, true},
		{"info", false, true, true, true},
		{"warn", false, false, true, true},
		{"error", false, false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.level, func(t *testing.T) {
			logger := New(&config.Config{LogLevel: tc.level})
			if logger == nil {
				t.Fatal("New() returned nil")
			}
			ctx := context.Background()
			if got := logger.Enabled(ctx, slog.LevelDebug); got != tc.wantDebug {
				t.Errorf("Enabled(Debug) = %v, want %v", got, tc.wantDebug)
			}
			if got := logger.Enabled(ctx, slog.LevelInfo); got != tc.wantInfo {
				t.Errorf("Enabled(Info) = %v, want %v", got, tc.wantInfo)
			}
			if got := logger.Enabled(ctx, slog.LevelWarn); got != tc.wantWarn {
				t.Errorf("Enabled(Warn) = %v, want %v", got, tc.wantWarn)
			}
			if got := logger.Enabled(ctx, slog.LevelError); got != tc.wantError {
				t.Errorf("Enabled(Error) = %v, want %v", got, tc.wantError)
			}
		})
	}
}

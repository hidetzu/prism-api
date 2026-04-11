// Package logging constructs the project's slog logger.
package logging

import (
	"log/slog"
	"os"
	"strings"

	"github.com/hidetzu/prism-api/internal/config"
)

// New returns a JSON-format slog logger configured at the level from cfg.
func New(cfg *config.Config) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(cfg.LogLevel),
	})
	return slog.New(handler)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

package logging

import (
	"log/slog"
	"os"
	"strings"
)

// Setup configures the default slog logger.
func Setup(level, env string) {
	lvl := parseLevel(level)
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: lvl}
	if env == "development" || env == "dev" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// With returns a logger with a component field.
func With(component string) *slog.Logger {
	return slog.Default().With("component", component)
}

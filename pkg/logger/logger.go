package logger

import (
	"log/slog"
	"os"
)

var Level = new(slog.LevelVar)

func New(level string, debug bool) *slog.Logger {
	switch level {
	case "debug":
		Level.Set(slog.LevelDebug)
	case "warn":
		Level.Set(slog.LevelWarn)
	case "error":
		Level.Set(slog.LevelError)
	default:
		Level.Set(slog.LevelInfo)
	}
	if debug {
		Level.Set(slog.LevelDebug)
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: Level}))
}

package pkg

import (
	"log/slog"
	"os"
)

// New returns a JSON logger at the given level. An empty or unrecognised
// level falls back to info rather than failing startup.
func New(level string) *slog.Logger {
	l := slog.LevelInfo
	if level != "" {
		if err := l.UnmarshalText([]byte(level)); err != nil {
			l = slog.LevelInfo
		}
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: l,
	}))
}

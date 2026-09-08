package logging

import (
	"io"
	"log/slog"
	"os"
)

// New returns a JSON logger at the given level, writing to stdout.
func New(level string) *slog.Logger {
	return newTo(os.Stdout, level)
}

// newTo is New with the destination pulled out, so a test can capture what it
// writes without New hardcoding os.Stdout.
//
// An empty level falls back to info silently; an unrecognised one also falls
// back to info, but not silently - it is almost always a typo, and finding
// out from a support ticket instead of the log itself is the wrong way to
// learn that.
func newTo(w io.Writer, level string) *slog.Logger {
	l := slog.LevelInfo
	unrecognised := false
	if level != "" {
		if err := l.UnmarshalText([]byte(level)); err != nil {
			l = slog.LevelInfo
			unrecognised = true
		}
	}
	logger := slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: l,
	}))
	if unrecognised {
		logger.Warn("Unrecognised LOG_LEVEL, defaulting to info", "log_level", level)
	}
	return logger
}

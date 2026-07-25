package logging

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

var (
	ErrUnsupportedLogFormat = errors.New("unsupported log format")
	ErrUnsupportedLogLevel  = errors.New("unsupported log level")
)

func New(level, format string) (*slog.Logger, error) {
	lvl, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	switch strings.ToLower(format) {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedLogFormat, format)
	}

	return slog.New(handler), nil
}

func parseLevel(level string) (slog.Leveler, error) {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedLogLevel, level)
	}
}

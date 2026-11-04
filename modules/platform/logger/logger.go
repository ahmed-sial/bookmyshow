// Package logger builds the application's structured logger and carries
// request-scoped loggers through context.Context.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Config controls how the application logger is built.
type Config struct {
	// Level is one of debug, info, warn, or error (case-insensitive).
	// An empty value defaults to info.
	Level string
	// Format is either "json" or "text". An empty value selects JSON in
	// production environments and text everywhere else.
	Format string
	// Env is the deployment environment name (for example "dev" or
	// "production"). It is only used to pick a default Format.
	Env string
}

// New builds a structured logger writing to w. A nil writer defaults to
// os.Stdout. It returns an error when Level or Format is not recognised so
// that misconfiguration fails fast instead of silently degrading.
func New(cfg Config, w io.Writer) (*slog.Logger, error) {
	if w == nil {
		w = os.Stdout
	}

	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	format, err := parseFormat(cfg.Format, cfg.Env)
	if err != nil {
		return nil, err
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}

	return slog.New(handler), nil
}

// MustNew is New that terminates the process when the configuration is
// invalid. It mirrors config.MustLoad.
func MustNew(cfg Config) *slog.Logger {
	l, err := New(cfg, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	return l
}

func parseLevel(s string) (slog.Level, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return slog.LevelInfo, nil
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return 0, fmt.Errorf("invalid log level %q: %w", s, err)
	}
	return level, nil
}

func parseFormat(format, env string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "":
		if isProduction(env) {
			return "json", nil
		}
		return "text", nil
	case "json":
		return "json", nil
	case "text":
		return "text", nil
	default:
		return "", fmt.Errorf("invalid log format %q: must be json or text", format)
	}
}

func isProduction(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production":
		return true
	default:
		return false
	}
}

type contextKey struct{}

// WithContext returns a copy of ctx that carries l.
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

// FromContext returns the logger stored in ctx. When ctx carries no logger it
// falls back to fallback, and then to slog.Default.
func FromContext(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if l, ok := ctx.Value(contextKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	if fallback != nil {
		return fallback
	}
	return slog.Default()
}

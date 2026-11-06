package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cfg       Config
		wantJSON  bool
		wantLevel slog.Level
		wantErr   bool
	}{
		{name: "empty config defaults to info", cfg: Config{Env: "dev"}, wantJSON: false, wantLevel: slog.LevelInfo},
		{name: "production default format is json", cfg: Config{Env: "production"}, wantJSON: true, wantLevel: slog.LevelInfo},
		{name: "explicit json format", cfg: Config{Format: "json", Env: "dev"}, wantJSON: true, wantLevel: slog.LevelInfo},
		{name: "explicit text format", cfg: Config{Format: "text", Env: "production"}, wantJSON: false, wantLevel: slog.LevelInfo},
		{name: "format comparison is case-insensitive", cfg: Config{Format: "JSON", Env: "dev"}, wantJSON: true, wantLevel: slog.LevelInfo},
		{name: "debug level", cfg: Config{Level: "debug", Format: "json"}, wantJSON: true, wantLevel: slog.LevelDebug},
		{name: "error level case-insensitive", cfg: Config{Level: "ERROR", Format: "text"}, wantJSON: false, wantLevel: slog.LevelError},
		{name: "empty level defaults to info", cfg: Config{Level: "", Format: "text"}, wantJSON: false, wantLevel: slog.LevelInfo},
		{name: "unknown level fails", cfg: Config{Level: "verbose", Format: "text"}, wantErr: true},
		{name: "unknown format fails", cfg: Config{Level: "info", Format: "xml"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			log, err := New(tt.cfg, &buf)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("New(%+v) built a logger, want an error", tt.cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("New(%+v): %v", tt.cfg, err)
			}

			// Log at exactly the level the config is expected to enable, and
			// give the handler a structured attribute to preserve.
			switch tt.wantLevel {
			case slog.LevelDebug:
				log.Debug("hello", "key", "value")
			case slog.LevelError:
				log.Error("hello", "key", "value")
			default:
				log.Info("hello", "key", "value")
			}
			output := buf.String()

			if tt.wantJSON {
				var entry map[string]any
				if err := json.Unmarshal([]byte(output), &entry); err != nil {
					t.Fatalf("output %q is not JSON: %v", output, err)
				}
				if entry["level"] != tt.wantLevel.String() {
					t.Errorf("level = %v, want %q", entry["level"], tt.wantLevel.String())
				}
				if entry["msg"] != "hello" {
					t.Errorf("msg = %v, want hello", entry["msg"])
				}
				if entry["key"] != "value" {
					t.Errorf("attrs not preserved: key = %v", entry["key"])
				}
				return
			}

			if got := output; len(got) == 0 {
				t.Fatal("no log output")
			}
		})
	}
}

func TestNewLevelFiltering(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	log, err := New(Config{Level: "warn", Format: "json"}, &buf)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	log.Info("dropped")
	if buf.Len() != 0 {
		t.Fatalf("info below the warn level was still written: %q", buf.String())
	}

	log.Warn("kept")
	if buf.Len() == 0 {
		t.Fatal("warn at the level threshold was filtered out")
	}
}

func TestNewNilWriterFallsBackToStdout(t *testing.T) {
	t.Parallel()

	// A nil writer must not panic; it defaults to os.Stdout.
	log, err := New(Config{Level: "error", Format: "text"}, nil)
	if err != nil {
		t.Fatalf("New with nil writer: %v", err)
	}
	log.Error("goes to stdout")
}

func TestLoggerContext(t *testing.T) {
	t.Parallel()

	t.Run("logger stored in context is returned", func(t *testing.T) {
		t.Parallel()

		stack := makeLogger()
		ctx := WithContext(context.Background(), stack)
		if got := FromContext(ctx, nil); got != stack {
			t.Fatalf("FromContext returned a different logger")
		}
	})

	t.Run("fallback is used when context is empty", func(t *testing.T) {
		t.Parallel()

		fallback := makeLogger()
		if got := FromContext(context.Background(), fallback); got != fallback {
			t.Fatal("fallback logger ignored")
		}
	})

	t.Run("slog default is the last resort", func(t *testing.T) {
		t.Parallel()

		if got := FromContext(context.Background(), nil); got != slog.Default() {
			t.Fatal("want slog.Default as the final fallback")
		}
	})
}

func makeLogger() *slog.Logger {
	l, err := New(Config{Level: "info", Format: "text"}, &bytes.Buffer{})
	if err != nil {
		panic(err) // invalid at compile time, safe for test setup
	}
	return l
}

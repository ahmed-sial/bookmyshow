package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func serveLogged(t *testing.T, requestID string, inner http.Handler) (*httptest.ResponseRecorder, *bytes.Buffer) {
	t.Helper()

	var buf bytes.Buffer
	requestLog := slog.New(slog.NewJSONHandler(&buf, nil))

	req := httptest.NewRequest(http.MethodGet, "/my/path", nil)
	if requestID != "" {
		req.Header.Set(RequestIDHeader, requestID)
	}
	rec := httptest.NewRecorder()

	RequestLogger(requestLog)(inner).ServeHTTP(rec, req)
	return rec, &buf
}

// requestIDObservingHandler writes the request id resolved into the context
// back to the client so tests can assert on both.
func requestIDObservingHandler(body *strings.Builder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body.WriteString(RequestID(r.Context()))
		w.WriteHeader(http.StatusTeapot)
	})
}

func TestRequestLogger(t *testing.T) {
	t.Parallel()

	t.Run("generates and echoes a request id", func(t *testing.T) {
		t.Parallel()

		var seen strings.Builder
		rec, _ := serveLogged(t, "", requestIDObservingHandler(&seen))

		if got := rec.Header().Get(RequestIDHeader); !uuidPattern.MatchString(got) {
			t.Fatalf("response header request id = %q, want a generated uuid", got)
		}
		if seen.String() != rec.Header().Get(RequestIDHeader) {
			t.Errorf("handler saw request id %q, header echoed %q", seen.String(), rec.Header().Get(RequestIDHeader))
		}
	})

	t.Run("reuses a recognizable inbound id", func(t *testing.T) {
		t.Parallel()

		var seen strings.Builder
		rec, _ := serveLogged(t, "frontend-42_a.b:c", requestIDObservingHandler(&seen))

		if got := rec.Header().Get(RequestIDHeader); got != "frontend-42_a.b:c" {
			t.Fatalf("header request id = %q, want the inbound value", got)
		}
		if seen.String() != "frontend-42_a.b:c" {
			t.Errorf("handler saw request id %q, want the inbound value", seen.String())
		}
	})

	badInbound := []struct {
		name string
		id   string
	}{
		{name: "header injection attempt", id: "<script>alert(1)</script>"},
		{name: "too long", id: strings.Repeat("a", 129)},
		{name: "non-ascii", id: "requête-42"},
		{name: "whitespace only", id: "   "},
	}

	for _, tt := range badInbound {
		t.Run("unusable inbound id is replaced: "+tt.name, func(t *testing.T) {
			t.Parallel()

			var seen strings.Builder
			rec, _ := serveLogged(t, tt.id, requestIDObservingHandler(&seen))

			got := rec.Header().Get(RequestIDHeader)
			if got == tt.id {
				t.Fatalf("echoed unusable request id %q", got)
			}
			if !uuidPattern.MatchString(got) {
				t.Fatalf("replacement request id = %q, want a generated uuid", got)
			}
			if seen.String() != got {
				t.Errorf("handler saw request id %q, header echoed %q", seen.String(), got)
			}
		})
	}

	t.Run("128-character id is accepted", func(t *testing.T) {
		t.Parallel()

		id := strings.Repeat("a", 128)
		var seen strings.Builder
		rec, _ := serveLogged(t, id, requestIDObservingHandler(&seen))

		if got := rec.Header().Get(RequestIDHeader); got != id {
			t.Fatalf("header request id = %q, want the inbound value", got)
		}
	})

	t.Run("logs one structured line per request", func(t *testing.T) {
		t.Parallel()

		rec, buf := serveLogged(t, "trace-1", requestIDObservingHandler(new(strings.Builder)))
		_ = rec

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 1 {
			t.Fatalf("logged %d lines, want 1: %q", len(lines), buf.String())
		}

		var entry map[string]any
		if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
			t.Fatalf("log line %q is not JSON: %v", lines[0], err)
		}

		if entry["msg"] != "http request" {
			t.Errorf("msg = %v, want http request", entry["msg"])
		}
		if entry["request_id"] != "trace-1" {
			t.Errorf("request_id = %v, want trace-1", entry["request_id"])
		}
		if entry["method"] != http.MethodGet {
			t.Errorf("method = %v, want GET", entry["method"])
		}
		if entry["path"] != "/my/path" {
			t.Errorf("path = %v, want /my/path", entry["path"])
		}
		if entry["status"] != float64(http.StatusTeapot) {
			t.Errorf("status = %v, want %d", entry["status"], http.StatusTeapot)
		}
		if _, ok := entry["duration_ms"]; !ok {
			t.Error("log entry has no duration_ms")
		}
		if _, ok := entry["bytes"]; !ok {
			t.Error("log entry has no bytes")
		}
	})

	t.Run("informational responses do not shadow the final status", func(t *testing.T) {
		t.Parallel()

		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusEarlyHints) // 1xx: not final
			w.WriteHeader(http.StatusNoContent)
		})

		_, buf := serveLogged(t, "", inner)

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 1 {
			t.Fatalf("logged %d lines, want 1: %q", len(lines), buf.String())
		}

		var entry map[string]any
		if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
			t.Fatalf("log line %q is not JSON: %v", lines[0], err)
		}
		if entry["status"] != float64(http.StatusNoContent) {
			t.Errorf("status = %v, want %d", entry["status"], http.StatusNoContent)
		}
	})
}

func TestResponseRecorder(t *testing.T) {
	t.Parallel()

	base := httptest.NewRecorder()
	rec := &responseRecorder{ResponseWriter: base}

	if got := rec.Unwrap(); got != base {
		t.Fatalf("Unwrap() = %v, want the wrapped writer", got)
	}

	rec.WriteHeader(http.StatusCreated)
	rec.WriteHeader(http.StatusInternalServerError) // must be ignored
	if rec.status != http.StatusCreated {
		t.Errorf("status = %d, want the first final status 201", rec.status)
	}
}

func TestRequestID(t *testing.T) {
	t.Parallel()

	t.Run("absent from context", func(t *testing.T) {
		t.Parallel()

		// RequestID must not panic when a handler runs without this
		// middleware in the chain.
		if got := RequestID(context.Background()); got != "" {
			t.Fatalf("RequestID = %q, want an empty string", got)
		}
	})
}

// Package middleware contains HTTP middleware shared by the modules.
package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ahmed-sial/bookmyshow/modules/platform/logger"
)

// RequestIDHeader is the header used to accept an inbound request ID and to
// return the resolved ID to the client.
const RequestIDHeader = "X-Request-ID"

type requestIDKey struct{}

// RequestID returns the request ID stored in ctx, or an empty string when the
// RequestLogger middleware did not run.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// RequestLogger returns middleware that:
//
//   - reuses a valid inbound X-Request-ID or generates a new UUID,
//   - echoes the ID back in the X-Request-ID response header,
//   - stores the ID and a request-scoped logger in the request context,
//   - logs one structured line per request with method, path, status,
//     response size, and duration.
//
// The request-scoped logger is retrievable with logger.FromContext.
func RequestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			requestID := sanitizeRequestID(r.Header.Get(RequestIDHeader))
			if requestID == "" {
				requestID = uuid.NewString()
			}
			w.Header().Set(RequestIDHeader, requestID)

			ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
			reqLog := log.With("request_id", requestID)
			ctx = logger.WithContext(ctx, reqLog)

			rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r.WithContext(ctx))

			reqLog.InfoContext(ctx, "http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"bytes", rec.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}

// sanitizeRequestID accepts only short, printable identifiers so that a
// client-supplied header cannot inject arbitrary content into logs. It
// returns "" when the value is missing or unusable.
func sanitizeRequestID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 128 {
		return ""
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == ':':
		default:
			return ""
		}
	}
	return id
}

// responseRecorder captures the status code and body size written by a
// handler while remaining transparent to http.ResponseController via Unwrap.
type responseRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (r *responseRecorder) WriteHeader(code int) {
	// Informational (1xx) responses are not final; the real status may follow.
	if code >= 100 && code < 200 {
		r.ResponseWriter.WriteHeader(code)
		return
	}
	if r.wroteHeader {
		return
	}
	r.status = code
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying ResponseWriter.
func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// corsTestOrigins are the origins the middleware under test is configured with.
var corsTestOrigins = []string{
	"https://app.bookmyshow.test",
	"https://admin.bookmyshow.test",
}

type corsProbe struct {
	called bool
}

// serveWithCors sends a request through CorsMiddleware with corsTestOrigins.
// The inner handler records whether it ran and answers 418 so its status is
// distinguishable from every status CorsMiddleware writes itself.
func serveWithCors(t *testing.T, method, origin string, headers map[string]string) (*httptest.ResponseRecorder, *corsProbe) {
	t.Helper()
	return serveWithCorsConfig(t, CorsConfig{AllowedOrigins: corsTestOrigins}, method, origin, headers)
}

func serveWithCorsConfig(t *testing.T, config CorsConfig, method, origin string, headers map[string]string) (*httptest.ResponseRecorder, *corsProbe) {
	t.Helper()

	probe := &corsProbe{}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probe.called = true
		w.WriteHeader(http.StatusTeapot)
	})

	req := httptest.NewRequest(method, "/api/v1/identity/some-id", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()

	CorsMiddleware(config)(inner).ServeHTTP(rec, req)
	return rec, probe
}

func TestCorsMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		method      string
		origin      string
		headers     map[string]string
		wantStatus  int
		wantHandler bool   // inner handler must run
		wantAllowOr string // Access-Control-Allow-Origin value; "" means absent
	}{
		{
			name:        "allowed origin, simple request reaches the handler",
			method:      http.MethodGet,
			origin:      corsTestOrigins[0],
			wantStatus:  http.StatusTeapot,
			wantHandler: true,
			wantAllowOr: corsTestOrigins[0],
		},
		{
			name:        "a later entry in the allowed list also passes",
			method:      http.MethodPost,
			origin:      corsTestOrigins[1],
			wantStatus:  http.StatusTeapot,
			wantHandler: true,
			wantAllowOr: corsTestOrigins[1],
		},
		{
			name:        "preflight from allowed origin answered without the handler",
			method:      http.MethodOptions,
			origin:      corsTestOrigins[0],
			headers:     map[string]string{"Access-Control-Request-Method": "POST"},
			wantStatus:  http.StatusNoContent,
			wantHandler: false,
			wantAllowOr: corsTestOrigins[0],
		},
		{
			name:        "disallowed origin is rejected",
			method:      http.MethodGet,
			origin:      "https://evil.example",
			wantStatus:  http.StatusForbidden,
			wantHandler: false,
		},
		{
			name:        "preflight from disallowed origin is rejected before the mux",
			method:      http.MethodOptions,
			origin:      "https://evil.example",
			headers:     map[string]string{"Access-Control-Request-Method": "DELETE"},
			wantStatus:  http.StatusForbidden,
			wantHandler: false,
		},
		{
			name:        "request with no Origin header is rejected (strict policy)",
			method:      http.MethodGet,
			wantStatus:  http.StatusForbidden,
			wantHandler: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec, probe := serveWithCors(t, tt.method, tt.origin, tt.headers)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if probe.called != tt.wantHandler {
				t.Errorf("inner handler ran = %v, want %v", probe.called, tt.wantHandler)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tt.wantAllowOr {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tt.wantAllowOr)
			}
		})
	}

	t.Run("no empty list of origins lets any tag through", func(t *testing.T) {
		t.Parallel()

		// An empty AllowedOrigins must behave like a permanent deny, even for
		// a request that looks like one of the usual test origins.
		rec, probe := serveWithCorsConfig(t, CorsConfig{}, http.MethodGet, corsTestOrigins[0], nil)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
		if probe.called {
			t.Error("inner handler ran with an empty origin list")
		}
	})
}

func TestCorsPreflightHeaders(t *testing.T) {
	t.Parallel()

	t.Run("preflight advertises methods, max age and echoes requested headers", func(t *testing.T) {
		t.Parallel()

		rec, probe := serveWithCors(t, http.MethodOptions, corsTestOrigins[0], map[string]string{
			"Access-Control-Request-Method":  "DELETE",
			"Access-Control-Request-Headers": "Authorization, Content-Type",
		})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", rec.Code)
		}
		if probe.called {
			t.Error("preflight reached the inner handler")
		}
		if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PUT, PATCH, DELETE, OPTIONS" {
			t.Errorf("Access-Control-Allow-Methods = %q", got)
		}
		if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type" {
			t.Errorf("Access-Control-Allow-Headers = %q, want the echoed request value", got)
		}
		if got := rec.Header().Get("Access-Control-Max-Age"); got != "600" {
			t.Errorf("Access-Control-Max-Age = %q, want 600", got)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("preflight body = %q, want empty", rec.Body.String())
		}
	})

	t.Run("preflight without requested headers falls back to the default set", func(t *testing.T) {
		t.Parallel()

		rec, _ := serveWithCors(t, http.MethodOptions, corsTestOrigins[0], map[string]string{
			"Access-Control-Request-Method": "GET",
		})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type" {
			t.Errorf("Access-Control-Allow-Headers = %q, want default authorization and content type", got)
		}
	})

	t.Run("rejected preflight exposes no CORS headers", func(t *testing.T) {
		t.Parallel()

		rec, _ := serveWithCors(t, http.MethodOptions, "https://evil.example", map[string]string{
			"Access-Control-Request-Method": "GET",
		})
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
		for _, header := range []string{
			"Access-Control-Allow-Origin",
			"Access-Control-Allow-Methods",
			"Access-Control-Allow-Headers",
			"Access-Control-Max-Age",
		} {
			if got := rec.Header().Get(header); got != "" {
				t.Errorf("%s = %q on a rejected preflight, want absent", header, got)
			}
		}
	})
}

func TestCorsDenialBody(t *testing.T) {
	t.Parallel()

	t.Run("denial is the standard JSON error envelope", func(t *testing.T) {
		t.Parallel()

		rec, _ := serveWithCors(t, http.MethodGet, "https://evil.example", nil)
		if got := errorField(t, rec); got != "origin not allowed" {
			t.Errorf("error = %q, want origin not allowed", got)
		}
		if got := rec.Header().Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
	})
}

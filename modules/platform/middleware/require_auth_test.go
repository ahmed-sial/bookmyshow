package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/ahmed-sial/bookmyshow/modules/platform/types"
)

// authTestSecret is shared by token generation and the middleware under test.
const authTestSecret = "auth-test-secret"

func hs256Token(t *testing.T, secret, subject string, exp time.Time) string {
	t.Helper()

	claims := &types.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("setup: sign token: %v", err)
	}
	return raw
}

type authProbe struct {
	id     uuid.UUID
	authed bool
}

// serveWithAuth sends a request through JWTMiddleware. The headerValue is set
// verbatim on the Authorization header (omit it to send no header). The probe
// records what the guarded handler saw in the context.
func serveWithAuth(t *testing.T, headerValue string) (*httptest.ResponseRecorder, *authProbe) {
	t.Helper()

	probe := &authProbe{}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probe.id, probe.authed = UserID(r.Context())
		w.WriteHeader(http.StatusTeapot)
	})
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	if headerValue != "" {
		req.Header.Set(AuthorizationHeader, headerValue)
	}
	rec := httptest.NewRecorder()

	JWTMiddleware(AuthConfig{JwtSecret: authTestSecret})(inner).ServeHTTP(rec, req)
	return rec, probe
}

func errorField(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response %q is not the expected error JSON: %v", rec.Body.String(), err)
	}
	return body.Error
}

func TestJWTMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("no Authorization header", func(t *testing.T) {
		t.Parallel()

		rec, probe := serveWithAuth(t, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if got := errorField(t, rec); got != "missing authentication token" {
			t.Fatalf("error = %q, want missing authentication token", got)
		}
		if probe.authed {
			t.Fatal("guarded handler ran without a token")
		}
	})

	malformed := []struct {
		name       string
		header     string
		wantStatus int
	}{
		{name: "non-bearer scheme", header: "Token " + "whatever"},
		{name: "bearer with no token", header: "Bearer"},
		{name: "bearer with extra fields", header: "Bearer two three"},
		{name: "empty bearer value", header: "Bearer "},
	}

	for _, tt := range malformed {
		t.Run("malformed header: "+tt.name, func(t *testing.T) {
			t.Parallel()

			rec, probe := serveWithAuth(t, tt.header)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if got := errorField(t, rec); got != "missing authentication token" {
				t.Fatalf("error = %q, want missing authentication token", got)
			}
			if probe.authed {
				t.Fatal("guarded handler ran with a malformed header")
			}
		})
	}

	t.Run("unparseable token", func(t *testing.T) {
		t.Parallel()

		rec, _ := serveWithAuth(t, "Bearer not-a-jwt")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if got := errorField(t, rec); got != "authentication failed" {
			t.Fatalf("error = %q, want authentication failed", got)
		}
	})

	t.Run("token signed with the wrong secret", func(t *testing.T) {
		t.Parallel()

		token := hs256Token(t, "not-the-secret", uuid.NewString(), time.Now().Add(time.Minute))
		rec, _ := serveWithAuth(t, "Bearer "+token)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if got := errorField(t, rec); got != "authentication failed" {
			t.Fatalf("error = %q, want authentication failed", got)
		}
	})

	t.Run("expired token points the client back to login", func(t *testing.T) {
		t.Parallel()

		token := hs256Token(t, authTestSecret, uuid.NewString(), time.Now().Add(-time.Minute))
		rec, probe := serveWithAuth(t, "Bearer "+token)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if got := errorField(t, rec); got != "token expired, please log in again" {
			t.Fatalf("error = %q, want token expired message", got)
		}
		if www := rec.Header().Get("WWW-Authenticate"); !strings.Contains(www, "invalid_token") {
			t.Errorf("WWW-Authenticate = %q, want invalid_token challenge", www)
		}
		if probe.authed {
			t.Fatal("expired token reached the guarded handler")
		}
	})

	t.Run("token without an expiration claim is rejected", func(t *testing.T) {
		t.Parallel()

		// Claims without exp. WithExpirationRequired must reject no-expiry
		// tokens, otherwise a client could mint long-lived identities.
		claims := &types.Claims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: uuid.NewString()},
		}
		raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(authTestSecret))
		if err != nil {
			t.Fatalf("setup: sign token: %v", err)
		}

		rec, _ := serveWithAuth(t, "Bearer "+raw)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if got := errorField(t, rec); got != "authentication failed" {
			t.Fatalf("error = %q, want authentication failed", got)
		}
	})

	t.Run("empty subject is rejected", func(t *testing.T) {
		t.Parallel()

		token := hs256Token(t, authTestSecret, "", time.Now().Add(time.Minute))
		rec, _ := serveWithAuth(t, "Bearer "+token)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("subject that is not a uuid is rejected", func(t *testing.T) {
		t.Parallel()

		token := hs256Token(t, authTestSecret, "definitely-not-a-uuid", time.Now().Add(time.Minute))
		rec, probe := serveWithAuth(t, "Bearer "+token)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if probe.authed {
			t.Fatal("non-uuid subject reached the guarded handler")
		}
	})

	t.Run("nil-uuid subject reaches the handler but is not an identity", func(t *testing.T) {
		t.Parallel()

		token := hs256Token(t, authTestSecret, uuid.Nil.String(), time.Now().Add(time.Minute))
		rec, probe := serveWithAuth(t, "Bearer "+token)
		if rec.Code != http.StatusTeapot {
			t.Fatalf("status = %d, want 418 (guarded handler)", rec.Code)
		}
		if probe.authed {
			t.Error("uuid.Nil subject accepted as an identity")
		}
		if probe.id != uuid.Nil {
			t.Errorf("id = %v, want uuid.Nil", probe.id)
		}
	})

	t.Run("validly signed RS256 token is rejected", func(t *testing.T) {
		t.Parallel()

		// Algorithm confusion guard: only HS256 tokens may pass.
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("setup: rsa key: %v", err)
		}
		claims := &types.Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   uuid.NewString(),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
			},
		}
		raw, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
		if err != nil {
			t.Fatalf("setup: sign token: %v", err)
		}

		rec, probe := serveWithAuth(t, "Bearer "+raw)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if probe.authed {
			t.Fatal("RS256 token reached the guarded handler")
		}
	})

	t.Run("valid token reaches the guarded handler", func(t *testing.T) {
		t.Parallel()

		id := uuid.New()
		token := hs256Token(t, authTestSecret, id.String(), time.Now().Add(time.Minute))
		rec, probe := serveWithAuth(t, "Bearer "+token)
		if rec.Code != http.StatusTeapot {
			t.Fatalf("status = %d, want 418 (guarded handler)", rec.Code)
		}
		if !probe.authed || probe.id != id {
			t.Fatalf("probe = %+v, want authenticated id %v", probe, id)
		}
	})
}

func TestUserID(t *testing.T) {
	t.Parallel()

	t.Run("context without an identity", func(t *testing.T) {
		t.Parallel()

		if _, ok := UserID(context.Background()); ok {
			t.Fatal("identity found in an unauthenticated context")
		}
	})

	t.Run("identity present", func(t *testing.T) {
		t.Parallel()

		id := uuid.New()
		ctx := context.WithValue(context.Background(), userIDKey{}, id)
		got, ok := UserID(ctx)
		if !ok || got != id {
			t.Fatalf("UserID = (%v, %v), want (%v, true)", got, ok, id)
		}
	})

	t.Run("nil uuid is not an identity", func(t *testing.T) {
		t.Parallel()

		ctx := context.WithValue(context.Background(), userIDKey{}, uuid.Nil)
		if _, ok := UserID(ctx); ok {
			t.Fatal("uuid.Nil accepted as an identity")
		}
	})

	t.Run("wrong value type is ignored", func(t *testing.T) {
		t.Parallel()

		ctx := context.WithValue(context.Background(), userIDKey{}, "not-a-uuid")
		if _, ok := UserID(ctx); ok {
			t.Fatal("string value accepted as an identity")
		}
	})
}

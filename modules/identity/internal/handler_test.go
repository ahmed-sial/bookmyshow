package identity

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/ahmed-sial/bookmyshow/modules/platform/config"
	"github.com/ahmed-sial/bookmyshow/modules/platform/middleware"
	"github.com/ahmed-sial/bookmyshow/modules/platform/types"
	"github.com/ahmed-sial/bookmyshow/modules/platform/validator"
)

// handlerSecret is the secret shared by the handler under test and its auth
// middleware within this file.
const handlerSecret = "handler-test-secret"

// routePath formats identity paths consistently across tests.
func routePath(suffix string) string {
	return "/api/v1/identity" + suffix
}

func newTestHandler() (*fakeIdentityRepository, http.Handler) {
	repo := newFakeIdentityRepository()
	svc := NewIdentityService(repo, validator.NewAppValidator(), &config.AppConfig{JwtSecret: handlerSecret})
	// Discard logs so intentional 500 paths do not pollute test output.
	h := NewHandler(svc, slog.New(slog.NewTextHandler(io.Discard, nil)))

	mux := http.NewServeMux()
	RegisterRoutes(mux, h, middleware.JWTMiddleware(middleware.AuthConfig{JwtSecret: handlerSecret}))
	return repo, mux
}

func signTestToken(t *testing.T, subject string) string {
	t.Helper()

	claims := &types.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(handlerSecret))
	if err != nil {
		t.Fatalf("setup: sign token: %v", err)
	}
	return signed
}

func doJSON(t *testing.T, mux http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("setup: marshal body: %v", err)
		}
		payload = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, payload)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set(middleware.AuthorizationHeader, "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response body %q is not a JSON object: %v", rec.Body.String(), err)
	}
	return out
}

// storeUser inserts a user directly into the fake repository with explicit
// email and phone so conflict tests can craft their own pairs.
func storeUser(t *testing.T, repo *fakeIdentityRepository, email, phone string) uuid.UUID {
	t.Helper()

	hash, err := hashPassword([]byte("Sup3rSecret"))
	if err != nil {
		t.Fatalf("setup: hashPassword: %v", err)
	}
	u := user{
		id:           uuid.New(),
		firstName:    "Ada",
		lastName:     "Lovelace",
		email:        email,
		passwordHash: string(hash),
		phoneNumber:  phone,
		status:       true,
	}
	repo.byID[u.id] = u
	repo.byEmail[u.email] = u
	repo.byPhone[u.phoneNumber] = u
	return u.id
}

func TestHandler_CreateUser(t *testing.T) {
	t.Parallel()

	t.Run("registers a user", func(t *testing.T) {
		t.Parallel()

		repo, mux := newTestHandler()

		rec := doJSON(t, mux, http.MethodPost, routePath("/auth/register"), "", validCreateUserDto())
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d (%s), want 201", rec.Code, rec.Body.String())
		}
		if len(repo.byEmail) != 1 {
			t.Fatalf("repository holds %d users, want 1", len(repo.byEmail))
		}
	})

	t.Run("invalid JSON body is rejected", func(t *testing.T) {
		t.Parallel()

		_, mux := newTestHandler()

		req := httptest.NewRequest(http.MethodPost, routePath("/auth/register"), strings.NewReader("{not json"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if got := decodeBody(t, rec); got["error"] != "invalid JSON body" {
			t.Fatalf("body = %v, want invalid JSON body error", got)
		}
	})

	t.Run("unknown fields are rejected", func(t *testing.T) {
		t.Parallel()

		_, mux := newTestHandler()

		raw := `{"firstName":"Ada","email":"ada@example.com","password":"Sup3rSecret","phoneNumber":"+15550001","admin":true}`
		req := httptest.NewRequest(http.MethodPost, routePath("/auth/register"), strings.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d (%s), want 400", rec.Code, rec.Body.String())
		}
	})

	t.Run("validation failures are unprocessable", func(t *testing.T) {
		t.Parallel()

		_, mux := newTestHandler()
		dto := validCreateUserDto()
		dto.FirstName = ""

		rec := doJSON(t, mux, http.MethodPost, routePath("/auth/register"), "", dto)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d (%s), want 422", rec.Code, rec.Body.String())
		}

		var body struct {
			Errors []map[string]string `json:"errors"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		found := false
		for _, fe := range body.Errors {
			if fe["field"] == "firstName" {
				found = true
			}
		}
		if !found {
			t.Fatalf("errors = %v, want a firstName failure", body.Errors)
		}
	})

	t.Run("duplicate email is a conflict", func(t *testing.T) {
		t.Parallel()

		repo, mux := newTestHandler()
		storeUser(t, repo, "taken@example.com", "+15550009")

		dto := validCreateUserDto()
		dto.Email = "taken@example.com"
		dto.PhoneNumber = "+15550010"

		rec := doJSON(t, mux, http.MethodPost, routePath("/auth/register"), "", dto)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d (%s), want 409", rec.Code, rec.Body.String())
		}
		if got := decodeBody(t, rec); got["error"] != "email already registered" {
			t.Fatalf("body = %v, want email already registered", got)
		}
	})

	t.Run("duplicate phone is a conflict", func(t *testing.T) {
		t.Parallel()

		repo, mux := newTestHandler()
		storeUser(t, repo, "other@example.com", "+15550009")

		dto := validCreateUserDto()
		dto.Email = "fresh@example.com"
		dto.PhoneNumber = "+1 555 0009"

		rec := doJSON(t, mux, http.MethodPost, routePath("/auth/register"), "", dto)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d (%s), want 409", rec.Code, rec.Body.String())
		}
		if got := decodeBody(t, rec); got["error"] != "phone number already registered" {
			t.Fatalf("body = %v, want phone number already registered", got)
		}
	})

	t.Run("unexpected errors return a generic 500 without leaking details", func(t *testing.T) {
		t.Parallel()

		repo, mux := newTestHandler()
		repo.createErr = errRepository

		rec := doJSON(t, mux, http.MethodPost, routePath("/auth/register"), "", validCreateUserDto())
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}
		got := decodeBody(t, rec)
		if got["error"] != "internal error" {
			t.Fatalf("body = %v, want generic internal error", got)
		}
		if body := rec.Body.String(); strings.Contains(body, "repository failure") {
			t.Errorf("response leaks internal error text: %s", body)
		}
	})
}

func TestHandler_Login(t *testing.T) {
	t.Parallel()

	loginPath := routePath("/auth/login")

	t.Run("logs in with valid credentials", func(t *testing.T) {
		t.Parallel()

		repo, mux := newTestHandler()
		id := storeUser(t, repo, "ada@example.com", "+15550001")

		rec := doJSON(t, mux, http.MethodPost, loginPath, "", UserLoginReqDto{Email: "ada@example.com", Password: "Sup3rSecret"})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
		}

		var out struct {
			Token       string `json:"token"`
			ID          string `json:"id"`
			FirstName   string `json:"firstName"`
			LastName    string `json:"lastName"`
			Email       string `json:"email"`
			PhoneNumber string `json:"phoneNumber"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Token == "" || out.ID != id.String() || out.FirstName != "Ada" {
			t.Fatalf("response = %+v, want token and user fields", out)
		}
	})

	tests := []struct {
		name       string
		email      string
		password   string
		wantStatus int
		wantError  string
	}{
		{name: "wrong password", email: "ada@example.com", password: "Wr0ngPass!", wantStatus: http.StatusUnauthorized, wantError: "invalid credentials"},
		{name: "unknown email", email: "ghost@example.com", password: "Sup3rSecret", wantStatus: http.StatusUnauthorized, wantError: "invalid credentials"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo, mux := newTestHandler()
			storeUser(t, repo, "ada@example.com", "+15550001")

			rec := doJSON(t, mux, http.MethodPost, loginPath, "", UserLoginReqDto{Email: tt.email, Password: tt.password})
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d (%s), want %d", rec.Code, rec.Body.String(), tt.wantStatus)
			}
			if got := decodeBody(t, rec); got["error"] != tt.wantError {
				t.Fatalf("body = %v, want error %q", got, tt.wantError)
			}
		})
	}
}

func TestHandler_FindByID(t *testing.T) {
	t.Parallel()

	t.Run("unauthenticated request is rejected", func(t *testing.T) {
		t.Parallel()

		_, mux := newTestHandler()

		rec := doJSON(t, mux, http.MethodGet, routePath("/"+uuid.NewString()), "", nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("malformed id is rejected", func(t *testing.T) {
		t.Parallel()

		_, mux := newTestHandler()
		id := uuid.New()

		rec := doJSON(t, mux, http.MethodGet, routePath("/not-a-uuid"), signTestToken(t, id.String()), nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d (%s), want 400", rec.Code, rec.Body.String())
		}
		if got := decodeBody(t, rec); got["error"] != "invalid uuid format" {
			t.Fatalf("body = %v, want invalid uuid format", got)
		}
	})

	t.Run("own record is returned", func(t *testing.T) {
		t.Parallel()

		repo, mux := newTestHandler()
		id := storeUser(t, repo, "ada@example.com", "+15550001")

		rec := doJSON(t, mux, http.MethodGet, routePath("/"+id.String()), signTestToken(t, id.String()), nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
		}

		var got struct {
			ID          string `json:"id"`
			FirstName   string `json:"firstName"`
			LastName    string `json:"lastName"`
			Email       string `json:"email"`
			PhoneNumber string `json:"phoneNumber"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got.ID != id.String() || got.FirstName != "Ada" || got.LastName != "Lovelace" ||
			got.Email != "ada@example.com" || got.PhoneNumber != "+15550001" {
			t.Fatalf("got %+v, want stored user fields", got)
		}
	})

	t.Run("another user is reported not found", func(t *testing.T) {
		t.Parallel()

		repo, mux := newTestHandler()
		storeUser(t, repo, "ada@example.com", "+15550001")
		viewerID := uuid.New()

		rec := doJSON(t, mux, http.MethodGet, routePath("/"+uuid.NewString()), signTestToken(t, viewerID.String()), nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d (%s), want 404", rec.Code, rec.Body.String())
		}
	})

	t.Run("repository errors return a generic 500", func(t *testing.T) {
		t.Parallel()

		repo, mux := newTestHandler()
		repo.findByIDErr = errRepository

		id := uuid.New()
		rec := doJSON(t, mux, http.MethodGet, routePath("/"+id.String()), signTestToken(t, id.String()), nil)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d (%s), want 500", rec.Code, rec.Body.String())
		}
		if got := decodeBody(t, rec); got["error"] != "internal error" {
			t.Fatalf("body = %v, want generic internal error", got)
		}
	})
}

func TestHandler_DeleteByID(t *testing.T) {
	t.Parallel()

	t.Run("own account is deleted", func(t *testing.T) {
		t.Parallel()

		repo, mux := newTestHandler()
		id := storeUser(t, repo, "ada@example.com", "+15550001")

		rec := doJSON(t, mux, http.MethodDelete, routePath("/"+id.String()), signTestToken(t, id.String()), nil)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d (%s), want 204", rec.Code, rec.Body.String())
		}
		if len(repo.byID) != 0 {
			t.Fatalf("repository still holds %d users after delete", len(repo.byID))
		}
	})

	t.Run("another user cannot be deleted", func(t *testing.T) {
		t.Parallel()

		repo, mux := newTestHandler()
		storeUser(t, repo, "ada@example.com", "+15550001")

		rec := doJSON(t, mux, http.MethodDelete, routePath("/"+uuid.NewString()), signTestToken(t, uuid.NewString()), nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d (%s), want 404", rec.Code, rec.Body.String())
		}
	})

	t.Run("unauthenticated request is rejected", func(t *testing.T) {
		t.Parallel()

		_, mux := newTestHandler()

		rec := doJSON(t, mux, http.MethodDelete, routePath("/"+uuid.NewString()), "", nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})
}

package httputil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusTeapot, map[string]any{"ok": true, "count": 3})

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTeapot)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not valid JSON: %v", rec.Body.String(), err)
	}
	if body["ok"] != true {
		t.Errorf("ok = %v, want true", body["ok"])
	}
	if body["count"] != float64(3) {
		t.Errorf("count = %v, want 3", body["count"])
	}
}

func TestWriteJSONStructPayload(t *testing.T) {
	t.Parallel()

	type user struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}

	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusOK, user{ID: "abc", Email: "a@example.com"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var u user
	if err := json.Unmarshal(rec.Body.Bytes(), &u); err != nil {
		t.Fatalf("body %q does not unmarshal into the payload type: %v", rec.Body.String(), err)
	}
	if u.ID != "abc" || u.Email != "a@example.com" {
		t.Fatalf("body = %+v, want the written struct", u)
	}
}

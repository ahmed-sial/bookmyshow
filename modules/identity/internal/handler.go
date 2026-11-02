package identity

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

type Handler struct {
	svc *IdentityService
}

func NewHandler(svc *IdentityService) *Handler { return &Handler{svc: svc} }

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB cap
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var dto CreateUserDto
	if err := dec.Decode(&dto); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	err := h.svc.CreateUser(r.Context(), dto)

	var ve *ValidationError
	switch {
	case err == nil:
		w.WriteHeader(http.StatusCreated)
	case errors.As(err, &ve):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"errors": ve.Fields})
	case errors.Is(err, ErrEmailTaken):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "email already registered"})
	case errors.Is(err, ErrPhoneTaken):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "phone number already registered"})
	default:
		log.Printf("create user: %v", err) // log details server-side only
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

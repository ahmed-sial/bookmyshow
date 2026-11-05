package identity

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/ahmed-sial/bookmyshow/modules/platform/httputil"
	"github.com/ahmed-sial/bookmyshow/modules/platform/logger"
	"github.com/ahmed-sial/bookmyshow/modules/platform/middleware"
)

type Handler struct {
	svc *IdentityService
	log *slog.Logger
}

func NewHandler(svc *IdentityService, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{svc: svc, log: log}
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context(), h.log)

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB cap
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var dto CreateUserDto
	if err := dec.Decode(&dto); err != nil {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	err := h.svc.CreateUser(r.Context(), dto)

	var ve *ValidationError
	switch {
	case err == nil:
		w.WriteHeader(http.StatusCreated)
	case errors.As(err, &ve):
		httputil.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"errors": ve.Fields})
	case errors.Is(err, ErrEmailTaken):
		httputil.WriteJSON(w, http.StatusConflict, map[string]string{"error": "email already registered"})
	case errors.Is(err, ErrPhoneTaken):
		httputil.WriteJSON(w, http.StatusConflict, map[string]string{"error": "phone number already registered"})
	default:
		log.Error("create user failed", "error", err)
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func (h *Handler) FindByID(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context(), h.log)
	viewerID, ok := middleware.UserID(r.Context())
	if !ok {
		httputil.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication failed"})
		return
	}

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid uuid format"})
		return
	}
	user, err := h.svc.FindByID(r.Context(), id, viewerID)
	switch {
	case err == nil:
		httputil.WriteJSON(w, http.StatusOK, user)
	case errors.Is(err, ErrUserNotFound):
		httputil.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
	default:
		log.Error("find user failed", "error", err, "user_id", id)
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context(), h.log)

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB cap
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var dto UserLoginReqDto
	if err := dec.Decode(&dto); err != nil {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	u, err := h.svc.Login(r.Context(), dto)
	var ve *ValidationError
	switch {
	case err == nil:
		httputil.WriteJSON(w, http.StatusOK, u)
	case errors.As(err, &ve):
		httputil.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"errors": ve.Fields})
	case errors.Is(err, ErrUserNotFound), errors.Is(err, ErrPasswordInvalid):
		httputil.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
	default:
		log.Error("login user failed", "error", err)
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func (h *Handler) DeleteByID(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context(), h.log)
	viewerID, ok := middleware.UserID(r.Context())
	if !ok {
		httputil.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication failed"})
		return
	}

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid uuid format"})
		return
	}

	err = h.svc.DeleteByID(r.Context(), id, viewerID)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrUserNotFound):
		httputil.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
	default:
		log.Error("delete user failed", "error", err, "user_id", id)
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

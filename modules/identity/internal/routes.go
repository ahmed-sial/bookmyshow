package identity

import "net/http"

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("POST /api/v1/identity/auth/register", h.CreateUser)
	mux.HandleFunc("GET /api/v1/identity/{id}", h.FindByID)
}

package identity

import "net/http"

type Middleware func(http.Handler) http.Handler

func RegisterRoutes(mux *http.ServeMux, h *Handler, auth Middleware) {
	// Public routes.
	mux.HandleFunc("POST /api/v1/identity/auth/register", h.CreateUser)
	mux.HandleFunc("POST /api/v1/identity/auth/login", h.Login)

	// Private routes, protected by JWT authentication.
	mux.Handle("GET /api/v1/identity/{id}", auth(http.HandlerFunc(h.FindByID)))
	mux.Handle("DELETE /api/v1/identity/{id}", auth(http.HandlerFunc(h.DeleteByID)))
}

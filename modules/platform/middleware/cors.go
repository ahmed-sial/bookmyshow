package middleware

import (
	"net/http"
	"slices"

	"github.com/ahmed-sial/bookmyshow/modules/platform/httputil"
)

type CorsConfig struct {
	AllowedOrigins []string
}

func CorsMiddleware(config CorsConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			isOriginAllowed := slices.Contains(config.AllowedOrigins, origin)

			if !isOriginAllowed {
				httputil.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "origin not allowed"})
				return
			}

			// Set base CORS headers for allowed origins.
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")

			// Handle CORS Preflight (OPTIONS) Requests properly
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")

				// Dynamically echo requested headers back to the browser
				if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
					w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
				} else {
					w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				}

				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// Forward standard requests to the next handler
			next.ServeHTTP(w, r)

		})
	}

}

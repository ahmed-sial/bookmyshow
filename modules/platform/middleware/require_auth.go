package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ahmed-sial/bookmyshow/modules/platform/types"
	"github.com/golang-jwt/jwt/v5"
)

const AuthorizationHeader = "Authorization"

type AuthConfig struct {
	JwtSecret string
}

type userIDKey struct{}

// UserID returns the authenticated user's ID stored in ctx by JWTMiddleware.
func UserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey{}).(string)
	return id, ok && id != ""
}

func JWTMiddleware(cfg AuthConfig) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authentication token"})
				return
			}
			claims, err := validateToken(token, cfg)
			if err != nil || claims == nil || claims.Subject == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication failed"})
				return
			}
			ctx := context.WithValue(r.Context(), userIDKey{}, claims.Subject)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearerToken(r *http.Request) string {
	header := r.Header.Get(AuthorizationHeader)
	if header == "" {
		return ""
	}
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return ""
	}
	return token
}

func validateToken(t string, cfg AuthConfig) (*types.Claims, error) {
	claims := &types.Claims{}
	token, err := jwt.ParseWithClaims(t, claims, func(t *jwt.Token) (any, error) {
		return []byte(cfg.JwtSecret), nil
	}, jwt.WithValidMethods([]string{"HS256"}))

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, nil
	}
	return claims, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

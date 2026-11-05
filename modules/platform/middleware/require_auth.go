package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/ahmed-sial/bookmyshow/modules/platform/httputil"
	"github.com/ahmed-sial/bookmyshow/modules/platform/types"
	"github.com/golang-jwt/jwt/v5"
)

const AuthorizationHeader = "Authorization"

type AuthConfig struct {
	JwtSecret string
}

type userIDKey struct{}

// UserID returns the authenticated user's ID stored in ctx by JWTMiddleware.
func UserID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDKey{}).(uuid.UUID)
	return id, ok && id != uuid.Nil
}

func JWTMiddleware(cfg AuthConfig) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				httputil.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authentication token"})
				return
			}
			claims, err := validateToken(token, cfg)
			if err != nil {
				// Expired tokens are still well-formed; tell the client to
				// authenticate again instead of returning a generic failure.
				if errors.Is(err, jwt.ErrTokenExpired) {
					w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token", error_description="token expired"`)
					httputil.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "token expired, please log in again"})
					return
				}
				httputil.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication failed"})
				return
			}
			if claims == nil || claims.Subject == "" {
				httputil.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication failed"})
				return
			}
			userID, err := uuid.Parse(claims.Subject)
			if err != nil {
				httputil.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication failed"})
				return
			}
			ctx := context.WithValue(r.Context(), userIDKey{}, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearerToken(r *http.Request) string {
	header := r.Header.Get(AuthorizationHeader)
	if header == "" {
		return ""
	}
	fields := strings.Fields(header)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return ""
	}
	return fields[1]
}

func validateToken(t string, cfg AuthConfig) (*types.Claims, error) {
	claims := &types.Claims{}
	token, err := jwt.ParseWithClaims(t, claims, func(t *jwt.Token) (any, error) {
		return []byte(cfg.JwtSecret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, nil
	}
	return claims, nil
}

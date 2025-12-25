package middleware

import (
	"context"
	"net/http"
	"strings"

	"orbo/internal/auth"
)

// ContextKey is a custom type for context keys
type ContextKey string

const (
	// UserContextKey is the key for storing user claims in context
	UserContextKey ContextKey = "user"
)

// AuthMiddleware creates an HTTP middleware for JWT authentication
func AuthMiddleware(authenticator *auth.Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth if disabled
			if !authenticator.IsEnabled() {
				next.ServeHTTP(w, r)
				return
			}

			// Extract token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error": "missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			// Check for Bearer prefix
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				http.Error(w, `{"error": "invalid authorization header format"}`, http.StatusUnauthorized)
				return
			}

			tokenString := parts[1]

			// Validate token
			claims, err := authenticator.ValidateToken(tokenString)
			if err != nil {
				if err == auth.ErrExpiredToken {
					http.Error(w, `{"error": "token has expired"}`, http.StatusUnauthorized)
				} else {
					http.Error(w, `{"error": "invalid token"}`, http.StatusUnauthorized)
				}
				return
			}

			// Add claims to context
			ctx := context.WithValue(r.Context(), UserContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserFromContext retrieves user claims from the request context
func GetUserFromContext(ctx context.Context) *auth.Claims {
	claims, ok := ctx.Value(UserContextKey).(*auth.Claims)
	if !ok {
		return nil
	}
	return claims
}

// RequireAuth is a convenience wrapper that returns 401 if user is not in context
func RequireAuth(ctx context.Context) (*auth.Claims, error) {
	claims := GetUserFromContext(ctx)
	if claims == nil {
		return nil, auth.ErrInvalidToken
	}
	return claims, nil
}

package services

import (
	"context"

	auth_service "orbo/gen/auth"
	"orbo/internal/auth"
	"orbo/internal/middleware"
)

// AuthImplementation implements the auth service
type AuthImplementation struct {
	authenticator *auth.Authenticator
}

// NewAuthService creates a new auth service implementation
func NewAuthService(authenticator *auth.Authenticator) auth_service.Service {
	return &AuthImplementation{
		authenticator: authenticator,
	}
}

// Login authenticates a user and returns a JWT token
func (a *AuthImplementation) Login(ctx context.Context, payload *auth_service.LoginPayload) (*auth_service.LoginResult, error) {
	token, expiresAt, err := a.authenticator.Authenticate(payload.Username, payload.Password)
	if err != nil {
		if err == auth.ErrInvalidCredentials {
			return nil, &auth_service.UnauthorizedError{Message: "Invalid username or password"}
		}
		if err == auth.ErrAuthDisabled {
			return nil, &auth_service.UnauthorizedError{Message: "Authentication is disabled"}
		}
		return nil, &auth_service.UnauthorizedError{Message: err.Error()}
	}

	return &auth_service.LoginResult{
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

// Status returns the current authentication status
func (a *AuthImplementation) Status(ctx context.Context) (*auth_service.StatusResult, error) {
	enabled := a.authenticator.IsEnabled()
	authenticated := false
	var username *string

	// Check if user is authenticated via middleware
	claims := middleware.GetUserFromContext(ctx)
	if claims != nil {
		authenticated = true
		username = &claims.Username
	}

	return &auth_service.StatusResult{
		Enabled:       enabled,
		Authenticated: authenticated,
		Username:      username,
	}, nil
}

package auth

import (
	"errors"
	"os"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAuthDisabled       = errors.New("authentication is disabled")
)

// Authenticator handles user authentication
type Authenticator struct {
	enabled      bool
	username     string
	passwordHash []byte
	jwtManager   *JWTManager
}

// NewAuthenticator creates a new authenticator from environment variables
func NewAuthenticator() *Authenticator {
	enabled := os.Getenv("AUTH_ENABLED") == "true"

	username := os.Getenv("AUTH_USERNAME")
	if username == "" {
		username = "admin"
	}

	password := os.Getenv("AUTH_PASSWORD")
	var passwordHash []byte

	if enabled && password != "" {
		// Check if password is already a bcrypt hash
		if len(password) == 60 && password[0] == '$' {
			passwordHash = []byte(password)
		} else {
			// Hash the plaintext password
			hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err == nil {
				passwordHash = hash
			}
		}
	}

	return &Authenticator{
		enabled:      enabled,
		username:     username,
		passwordHash: passwordHash,
		jwtManager:   NewJWTManager(),
	}
}

// IsEnabled returns whether authentication is enabled
func (a *Authenticator) IsEnabled() bool {
	return a.enabled
}

// Authenticate validates credentials and returns a JWT token
func (a *Authenticator) Authenticate(username, password string) (string, int64, error) {
	if !a.enabled {
		return "", 0, ErrAuthDisabled
	}

	if username != a.username {
		return "", 0, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword(a.passwordHash, []byte(password)); err != nil {
		return "", 0, ErrInvalidCredentials
	}

	token, expiresAt, err := a.jwtManager.GenerateToken(username)
	if err != nil {
		return "", 0, err
	}

	return token, expiresAt.Unix(), nil
}

// ValidateToken validates a JWT token
func (a *Authenticator) ValidateToken(token string) (*Claims, error) {
	return a.jwtManager.ValidateToken(token)
}

// JWTManager returns the JWT manager
func (a *Authenticator) JWTManager() *JWTManager {
	return a.jwtManager
}

// HashPassword creates a bcrypt hash of a password (utility function)
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

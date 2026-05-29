package middleware

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
)

// JWTCustomClaims are custom claims extending default ones.
type JWTCustomClaims struct {
	UserID uuid.UUID `json:"user_id"`
	Role   string    `json:"role"`
	jwt.RegisteredClaims
}

// GetJWTConfig returns the configuration for the JWT middleware.
// In a real application, the secret key should be loaded from environment variables.
func GetJWTConfig() echojwt.Config {
	// Hardcoded for development/testing purposes based on the plan.
	// Must be moved to env vars for production.
	return echojwt.Config{
		NewClaimsFunc: func(c echo.Context) jwt.Claims {
			return new(JWTCustomClaims)
		},
		SigningKey: []byte("super-secret-key-for-local-dev"),
	}
}

// GetUserIDFromContext extracts the user UUID securely from the Echo context.
func GetUserIDFromContext(c echo.Context) (uuid.UUID, error) {
	user := c.Get("user")
	if user == nil {
		return uuid.Nil, errors.New("user context not found")
	}

	token, ok := user.(*jwt.Token)
	if !ok {
		return uuid.Nil, errors.New("invalid jwt token format in context")
	}

	claims, ok := token.Claims.(*JWTCustomClaims)
	if !ok {
		return uuid.Nil, errors.New("invalid jwt claims format")
	}

	return claims.UserID, nil
}

package middleware

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
)

const jwtTokenLookup = "header:Authorization:Bearer ,cookie:sevenoxcloud.auth_token,cookie:auth_token,cookie:access_token"

// JWTCustomClaims are custom claims extending default ones.
type JWTCustomClaims struct {
	UserID   uuid.UUID `json:"user_id"`
	TenantID string    `json:"tenant_id,omitempty"`
	Role     string    `json:"role"`
	jwt.RegisteredClaims
}

// GetJWTConfig returns the configuration for the JWT middleware.
func GetJWTConfig(signingKey []byte) echojwt.Config {
	return echojwt.Config{
		NewClaimsFunc: func(c echo.Context) jwt.Claims {
			return new(JWTCustomClaims)
		},
		SigningKey:  signingKey,
		TokenLookup: jwtTokenLookup,
	}
}

// GetUserIDFromContext extracts the user UUID securely from the Echo context.
func GetUserIDFromContext(c echo.Context) (uuid.UUID, error) {
	claims, err := jwtClaimsFromContext(c)
	if err != nil {
		return uuid.Nil, err
	}

	return claims.UserID, nil
}

func jwtClaimsFromContext(c echo.Context) (*JWTCustomClaims, error) {
	user := c.Get("user")
	if user == nil {
		return nil, errors.New("user context not found")
	}

	token, ok := user.(*jwt.Token)
	if !ok {
		return nil, errors.New("invalid jwt token format in context")
	}

	claims, ok := token.Claims.(*JWTCustomClaims)
	if !ok {
		return nil, errors.New("invalid jwt claims format")
	}

	return claims, nil
}

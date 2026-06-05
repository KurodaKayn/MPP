package handlers

import (
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

// MockLogin creates a token for a given username. This is for local development only.
func (h *AuthHandler) MockLogin(c echo.Context) error {
	type LoginRequest struct {
		Username string `json:"username"`
	}

	req := new(LoginRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	var user models.User
	if err := h.db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		return sendError(c, http.StatusNotFound, "not_found", "user not found")
	}

	// Create JWT token
	claims := &middleware.JWTCustomClaims{
		UserID: user.ID,
		Role:   "user", // simple mock role
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 72)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	t, err := token.SignedString(h.jwtSigningKey)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", "failed to sign token")
	}

	return c.JSON(http.StatusOK, echo.Map{
		"token": t,
	})
}

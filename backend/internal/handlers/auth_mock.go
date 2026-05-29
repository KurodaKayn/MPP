package handlers

import (
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/middleware"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/models"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type AuthHandler struct {
	db *gorm.DB
}

func NewAuthHandler(db *gorm.DB) *AuthHandler {
	return &AuthHandler{db: db}
}

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

	// Encode token (using the same hardcoded key from middleware for dev)
	t, err := token.SignedString([]byte("super-secret-key-for-local-dev"))
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", "failed to sign token")
	}

	return c.JSON(http.StatusOK, echo.Map{
		"token": t,
	})
}

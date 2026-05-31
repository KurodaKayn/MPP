package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type LoginRequest struct {
	Username string `json:"username"`
}

type AuthResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

func (h *AuthHandler) Login(c echo.Context) error {
	req := new(LoginRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	if req.Username == "" {
		return sendError(c, http.StatusBadRequest, "invalid_request", "username is required")
	}

	var user models.User
	err := h.db.Where("username = ?", req.Username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Auto-create user if not found (dev-friendly mode)
			user = models.User{
				Username: req.Username,
				Role:     "user",
			}
			if err := h.db.Create(&user).Error; err != nil {
				return sendError(c, http.StatusInternalServerError, "internal_error", "failed to create user")
			}
		} else {
			return sendError(c, http.StatusInternalServerError, "internal_error", "database error")
		}
	}

	token, err := h.generateToken(&user)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", "failed to generate token")
	}

	return c.JSON(http.StatusOK, AuthResponse{
		Token:    token,
		Username: user.Username,
	})
}

func (h *AuthHandler) generateToken(user *models.User) (string, error) {
	claims := &middleware.JWTCustomClaims{
		UserID: user.ID,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 72)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(h.jwtSigningKey)
}

package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token    string    `json:"token"`
	UserID   uuid.UUID `json:"user_id"`
	Username string    `json:"username"`
}

func (h *AuthHandler) Register(c echo.Context) error {
	req := new(RegisterRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	if req.Username == "" || req.Password == "" {
		return sendError(c, http.StatusBadRequest, "invalid_request", "username and password are required")
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", "failed to hash password")
	}

	user := &models.User{
		ID:       uuid.New(),
		Username: req.Username,
		Password: string(hashedPassword),
		Role:     "user",
	}

	if err := h.db.Create(user).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return sendError(c, http.StatusConflict, "conflict", "username already exists")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", "failed to create user")
	}

	token, err := h.generateToken(user)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", "failed to generate token")
	}

	return c.JSON(http.StatusCreated, AuthResponse{
		Token:    token,
		UserID:   user.ID,
		Username: user.Username,
	})
}

func (h *AuthHandler) Login(c echo.Context) error {
	req := new(LoginRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	var user models.User
	if err := h.db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusUnauthorized, "unauthorized", "invalid username or password")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", "database error")
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", "invalid username or password")
	}

	token, err := h.generateToken(&user)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", "failed to generate token")
	}

	return c.JSON(http.StatusOK, AuthResponse{
		Token:    token,
		UserID:   user.ID,
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

package handlers

import (
	"errors"
	"net/http"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

func (h *AuthHandler) Register(c echo.Context) error {
	req := new(RegisterRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	if req.Username == "" || req.Password == "" {
		return sendError(c, http.StatusBadRequest, "invalid_request", "username and password are required")
	}

	if len(req.Password) < 8 {
		return sendError(c, http.StatusBadRequest, "invalid_request", "password must be at least 8 characters")
	}

	var hasUpper, hasLower bool
	for _, char := range req.Password {
		if unicode.IsUpper(char) {
			hasUpper = true
		}
		if unicode.IsLower(char) {
			hasLower = true
		}
	}
	if !hasUpper || !hasLower {
		return sendError(c, http.StatusBadRequest, "invalid_request", "password must contain at least one uppercase and one lowercase letter")
	}

	// Check if user already exists
	var existingUser models.User
	err := h.db.Where("username = ?", req.Username).First(&existingUser).Error
	if err == nil {
		return sendError(c, http.StatusConflict, "user_exists", "username already exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return sendError(c, http.StatusInternalServerError, "internal_error", "database error")
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", "failed to hash password")
	}

	// Create user
	newUser := models.User{
		Username:     req.Username,
		PasswordHash: string(hashedPassword),
		Role:         "user",
	}

	if err := h.db.Create(&newUser).Error; err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", "failed to create user")
	}

	// Generate token for auto-login after register
	token, err := h.generateToken(&newUser)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", "failed to generate token")
	}

	return c.JSON(http.StatusCreated, AuthResponse{
		Token:    token,
		Username: newUser.Username,
	})
}

func (h *AuthHandler) Login(c echo.Context) error {
	req := new(LoginRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	if req.Username == "" || req.Password == "" {
		return sendError(c, http.StatusBadRequest, "invalid_request", "username and password are required")
	}

	if !h.usernameLoginEnabled {
		return sendError(c, http.StatusUnauthorized, "invalid_credentials", "username login is disabled")
	}

	var user models.User
	err := h.db.Where("username = ?", req.Username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", "database error")
	}

	// Compare password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return sendError(c, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
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

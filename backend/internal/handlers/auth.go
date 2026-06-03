package handlers

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/email"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthHandler struct {
	db                   *gorm.DB
	redis                *redis.Client
	email                email.EmailService
	jwtSigningKey        []byte
	usernameLoginEnabled bool
}

func NewAuthHandler(db *gorm.DB, redis *redis.Client, email email.EmailService, jwtSigningKey []byte) *AuthHandler {
	return &AuthHandler{
		db:            db,
		redis:         redis,
		email:         email,
		jwtSigningKey: jwtSigningKey,
	}
}

func (h *AuthHandler) SetUsernameLoginEnabled(enabled bool) {
	h.usernameLoginEnabled = enabled
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Code     string `json:"code"`
}

type SendCodeRequest struct {
	Email string `json:"email"`
	Scene string `json:"scene"` // "register" or "forgot_password"
}

type ResetPasswordRequest struct {
	Email    string `json:"email"`
	Code     string `json:"code"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

func (h *AuthHandler) SendCode(c echo.Context) error {
	req := new(SendCodeRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	if req.Email == "" || req.Scene == "" {
		return sendError(c, http.StatusBadRequest, "invalid_request", "email and scene are required")
	}

	// Rate limit check: only allow sending code once every 60 seconds
	lastSendKey := fmt.Sprintf("auth:last_send:%s:%s", req.Scene, req.Email)
	if h.redis != nil {
		exists, err := h.redis.Exists(c.Request().Context(), lastSendKey).Result()
		if err == nil && exists > 0 {
			return sendError(c, http.StatusTooManyRequests, "rate_limited", "please wait 60 seconds before requesting another code")
		}
	}

	// Scene-specific checks
	if req.Scene == "register" {
		var count int64
		h.db.Model(&models.User{}).Where("email = ?", req.Email).Count(&count)
		if count > 0 {
			return sendError(c, http.StatusConflict, "email_exists", "email already registered")
		}
	} else if req.Scene == "forgot_password" {
		var count int64
		h.db.Model(&models.User{}).Where("email = ?", req.Email).Count(&count)
		if count == 0 {
			return sendError(c, http.StatusNotFound, "user_not_found", "no user found with this email")
		}
	} else {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid scene")
	}

	// Generate 6-digit code
	code := h.generateRandomCode(6)

	// Store code in Redis (valid for 10 minutes)
	if h.redis != nil {
		codeKey := fmt.Sprintf("auth:code:%s:%s", req.Scene, req.Email)
		err := h.redis.Set(c.Request().Context(), codeKey, code, 10*time.Minute).Err()
		if err != nil {
			return sendError(c, http.StatusInternalServerError, "internal_error", "failed to store code")
		}
		// Set rate limit key
		h.redis.Set(c.Request().Context(), lastSendKey, "1", 60*time.Second)
	} else {
		// If redis is nil (local dev without redis), we might want to log it or something
		fmt.Printf("DEBUG: Verification code for %s (%s): %s\n", req.Email, req.Scene, code)
	}

	// Send email
	if h.email != nil {
		var err error
		if req.Scene == "register" {
			err = h.email.SendVerificationCode(req.Email, code)
		} else {
			err = h.email.SendPasswordResetCode(req.Email, code)
		}
		if err != nil {
			return sendError(c, http.StatusInternalServerError, "internal_error", "failed to send email")
		}
	}

	return c.JSON(http.StatusOK, echo.Map{"message": "verification code sent"})
}

func (h *AuthHandler) ResetPassword(c echo.Context) error {
	req := new(ResetPasswordRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	if req.Email == "" || req.Code == "" || req.Password == "" {
		return sendError(c, http.StatusBadRequest, "invalid_request", "email, code and password are required")
	}

	if len(req.Password) < 8 {
		return sendError(c, http.StatusBadRequest, "invalid_request", "password must be at least 8 characters")
	}

	// Verify code
	if h.redis != nil {
		codeKey := fmt.Sprintf("auth:code:forgot_password:%s", req.Email)
		savedCode, err := h.redis.Get(c.Request().Context(), codeKey).Result()
		if err != nil || savedCode != req.Code {
			return sendError(c, http.StatusUnauthorized, "invalid_code", "invalid or expired verification code")
		}
		// Delete code after use
		h.redis.Del(c.Request().Context(), codeKey)
	}

	// Find user
	var user models.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		return sendError(c, http.StatusNotFound, "user_not_found", "user not found")
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", "failed to hash password")
	}

	// Update password
	user.PasswordHash = string(hashedPassword)
	if err := h.db.Save(&user).Error; err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", "failed to update password")
	}

	return c.JSON(http.StatusOK, echo.Map{"message": "password reset successfully"})
}

func (h *AuthHandler) generateRandomCode(length int) string {
	const charset = "0123456789"
	b := make([]byte, length)
	for i := range b {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[idx.Int64()]
	}
	return string(b)
}

func (h *AuthHandler) Register(c echo.Context) error {
	req := new(RegisterRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	if req.Username == "" || req.Email == "" || req.Password == "" || req.Code == "" {
		return sendError(c, http.StatusBadRequest, "invalid_request", "username, email, password and code are required")
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

	// Verify code
	if h.redis != nil {
		codeKey := fmt.Sprintf("auth:code:register:%s", req.Email)
		savedCode, err := h.redis.Get(c.Request().Context(), codeKey).Result()
		if err != nil || savedCode != req.Code {
			return sendError(c, http.StatusUnauthorized, "invalid_code", "invalid or expired verification code")
		}
		// Delete code after use
		h.redis.Del(c.Request().Context(), codeKey)
	}

	// Check if username or email already exists
	var existingUser models.User
	err := h.db.Where("username = ? OR email = ?", req.Username, req.Email).First(&existingUser).Error
	if err == nil {
		if existingUser.Username == req.Username {
			return sendError(c, http.StatusConflict, "user_exists", "username already exists")
		}
		return sendError(c, http.StatusConflict, "email_exists", "email already exists")
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
		Username:        req.Username,
		Email:           req.Email,
		IsEmailVerified: true, // If code is verified, email is verified
		PasswordHash:    string(hashedPassword),
		Role:            "user",
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

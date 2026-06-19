package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/rediskey"
	"github.com/kurodakayn/mpp-backend/internal/services/email"
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

const (
	verificationCodeTTL     = 10 * time.Minute
	verificationAttemptTTL  = 10 * time.Minute
	verificationMaxAttempts = 5
)

func (h *AuthHandler) SendCode(c echo.Context) error {
	req := new(SendCodeRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	if req.Email == "" || req.Scene == "" {
		return sendError(c, http.StatusBadRequest, "invalid_request", "email and scene are required")
	}
	req.Email = canonicalVerificationEmail(req.Email)
	if req.Email == "" {
		return sendError(c, http.StatusBadRequest, "invalid_request", "email and scene are required")
	}

	// Scene-specific checks
	switch req.Scene {
	case "register":
		var count int64
		h.db.Model(&models.User{}).Where("LOWER(email) = LOWER(?)", req.Email).Count(&count)
		if count > 0 {
			return sendError(c, http.StatusConflict, "email_exists", "email already registered")
		}
	case "forgot_password":
		var count int64
		h.db.Model(&models.User{}).Where("LOWER(email) = LOWER(?)", req.Email).Count(&count)
		if count == 0 {
			return sendError(c, http.StatusNotFound, "user_not_found", "no user found with this email")
		}
	default:
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid scene")
	}

	if h.redis == nil {
		return sendError(c, http.StatusServiceUnavailable, "verification_unavailable", "verification code service unavailable")
	}

	// Rate limit check: only allow sending code once every 60 seconds
	lastSendKey := verificationLastSendKey(req.Scene, req.Email)
	exists, err := h.redis.Exists(c.Request().Context(), lastSendKey).Result()
	if err != nil {
		return sendError(c, http.StatusServiceUnavailable, "verification_unavailable", "verification code service unavailable")
	}
	if exists > 0 {
		return sendError(c, http.StatusTooManyRequests, "rate_limited", "please wait 60 seconds before requesting another code")
	}

	// Generate 6-digit code
	code, err := h.generateRandomCode(6)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", "failed to generate verification code")
	}

	// Store code in Redis (valid for 10 minutes)
	codeKey := verificationCodeKey(req.Scene, req.Email)
	attemptKey := verificationAttemptKey(req.Scene, req.Email)
	if err := h.redis.Set(c.Request().Context(), codeKey, code, verificationCodeTTL).Err(); err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", "failed to store code")
	}
	if err := h.redis.Del(c.Request().Context(), attemptKey).Err(); err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", "failed to reset verification attempts")
	}
	if err := h.redis.Set(c.Request().Context(), lastSendKey, "1", 60*time.Second).Err(); err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", "failed to store rate limit")
	}

	// Send email
	if h.email != nil {
		ctx := c.Request().Context()
		var err error
		if req.Scene == "register" {
			err = h.email.SendVerificationCode(ctx, req.Email, code)
		} else {
			err = h.email.SendPasswordResetCode(ctx, req.Email, code)
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
	req.Email = canonicalVerificationEmail(req.Email)
	if req.Email == "" {
		return sendError(c, http.StatusBadRequest, "invalid_request", "email, code and password are required")
	}

	if len(req.Password) < 8 {
		return sendError(c, http.StatusBadRequest, "invalid_request", "password must be at least 8 characters")
	}

	if err := h.verifyCode(c, "forgot_password", req.Email, req.Code); err != nil {
		return err
	}

	// Find user
	var user models.User
	if err := h.db.Where("LOWER(email) = LOWER(?)", req.Email).First(&user).Error; err != nil {
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

func (h *AuthHandler) generateRandomCode(length int) (string, error) {
	const charset = "0123456789"
	b := make([]byte, length)
	for i := range b {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[idx.Int64()]
	}
	return string(b), nil
}

func verificationCodeKey(scene, email string) string {
	return fmt.Sprintf("auth:code:%s:%s", rediskey.Tag("email", verificationEmailKeyDigest(email)), rediskey.Part(scene))
}

func verificationAttemptKey(scene, email string) string {
	return fmt.Sprintf("auth:code_attempts:%s:%s", rediskey.Tag("email", verificationEmailKeyDigest(email)), rediskey.Part(scene))
}

func verificationLastSendKey(scene, email string) string {
	return fmt.Sprintf("auth:last_send:%s:%s", rediskey.Tag("email", verificationEmailKeyDigest(email)), rediskey.Part(scene))
}

func canonicalVerificationEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func verificationEmailKeyDigest(email string) string {
	sum := sha256.Sum256([]byte(canonicalVerificationEmail(email)))
	return hex.EncodeToString(sum[:])
}

func (h *AuthHandler) verifyCode(c echo.Context, scene, email, code string) error {
	if h.redis == nil {
		return sendError(c, http.StatusServiceUnavailable, "verification_unavailable", "verification code service unavailable")
	}

	ctx := c.Request().Context()
	codeKey := verificationCodeKey(scene, email)
	attemptKey := verificationAttemptKey(scene, email)

	savedCode, err := h.redis.Get(ctx, codeKey).Result()
	if err != nil || savedCode != code {
		attempts, incrErr := h.redis.Incr(ctx, attemptKey).Result()
		if incrErr != nil {
			return sendError(c, http.StatusServiceUnavailable, "verification_unavailable", "verification code service unavailable")
		}
		if attempts == 1 {
			if expireErr := h.redis.Expire(ctx, attemptKey, verificationAttemptTTL).Err(); expireErr != nil {
				return sendError(c, http.StatusServiceUnavailable, "verification_unavailable", "verification code service unavailable")
			}
		}
		if attempts >= verificationMaxAttempts {
			if delErr := h.redis.Del(ctx, codeKey, attemptKey).Err(); delErr != nil {
				return sendError(c, http.StatusServiceUnavailable, "verification_unavailable", "verification code service unavailable")
			}
		}
		return sendError(c, http.StatusUnauthorized, "invalid_code", "invalid or expired verification code")
	}

	if err := h.redis.Del(ctx, codeKey, attemptKey).Err(); err != nil {
		return sendError(c, http.StatusServiceUnavailable, "verification_unavailable", "verification code service unavailable")
	}
	return nil
}

func (h *AuthHandler) Register(c echo.Context) error {
	req := new(RegisterRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	if req.Username == "" || req.Email == "" || req.Password == "" || req.Code == "" {
		return sendError(c, http.StatusBadRequest, "invalid_request", "username, email, password and code are required")
	}
	req.Email = canonicalVerificationEmail(req.Email)
	if req.Email == "" {
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

	// Check if username or email already exists
	var existingUser models.User
	err := h.db.Where("username = ? OR LOWER(email) = LOWER(?)", req.Username, req.Email).First(&existingUser).Error
	if err == nil {
		if existingUser.Username == req.Username {
			return sendError(c, http.StatusConflict, "user_exists", "username already exists")
		}
		return sendError(c, http.StatusConflict, "email_exists", "email already exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return sendError(c, http.StatusInternalServerError, "internal_error", "database error")
	}

	if err := h.verifyCode(c, "register", req.Email, req.Code); err != nil {
		return err
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

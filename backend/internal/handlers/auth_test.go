package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/email"
)

func createTestUser(t *testing.T, db *gorm.DB, username, email, password string) models.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err)

	user := models.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		Role:         "user",
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func setupMiniRedis(t *testing.T) *redis.Client {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func storeVerificationCode(t *testing.T, rdb *redis.Client, scene, email, code string) {
	t.Helper()
	require.NoError(t, rdb.Set(context.Background(), fmt.Sprintf("auth:code:%s:%s", scene, email), code, 0).Err())
}

func TestSendCode(t *testing.T) {
	db := setupHandlerTestDB(t)
	rdb := setupMiniRedis(t)
	mockEmail := &email.MockEmailService{}
	handler := NewAuthHandler(db, rdb, mockEmail, []byte("test-secret"))

	e := echo.New()

	t.Run("Register_Success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/send-code", strings.NewReader(`{"email":"new@example.com", "scene":"register"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		require.NoError(t, handler.SendCode(e.NewContext(req, rec)))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "new@example.com", mockEmail.LastTo)
		assert.Equal(t, "MPP Registration Verification Code", mockEmail.LastSubject)
		assert.Len(t, mockEmail.LastBody, 6)
	})

	t.Run("Register_RateLimited", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/send-code", strings.NewReader(`{"email":"limited@example.com", "scene":"register"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		require.NoError(t, handler.SendCode(e.NewContext(req, rec)))
		require.Equal(t, http.StatusOK, rec.Code)

		req = httptest.NewRequest(http.MethodPost, "/api/auth/send-code", strings.NewReader(`{"email":"limited@example.com", "scene":"register"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec = httptest.NewRecorder()
		require.NoError(t, handler.SendCode(e.NewContext(req, rec)))
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	})

	t.Run("RequiresRedis", func(t *testing.T) {
		handlerWithoutRedis := NewAuthHandler(db, nil, mockEmail, []byte("test-secret"))
		req := httptest.NewRequest(http.MethodPost, "/api/auth/send-code", strings.NewReader(`{"email":"redis-required@example.com", "scene":"register"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		require.NoError(t, handlerWithoutRedis.SendCode(e.NewContext(req, rec)))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("Register_EmailExists", func(t *testing.T) {
		createTestUser(t, db, "user1", "user1@example.com", "Pass1234")
		req := httptest.NewRequest(http.MethodPost, "/api/auth/send-code", strings.NewReader(`{"email":"user1@example.com", "scene":"register"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		require.NoError(t, handler.SendCode(e.NewContext(req, rec)))
		assert.Equal(t, http.StatusConflict, rec.Code)
	})

	t.Run("ForgotPassword_Success", func(t *testing.T) {
		createTestUser(t, db, "user2", "user2@example.com", "Pass1234")
		req := httptest.NewRequest(http.MethodPost, "/api/auth/send-code", strings.NewReader(`{"email":"user2@example.com", "scene":"forgot_password"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		require.NoError(t, handler.SendCode(e.NewContext(req, rec)))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "user2@example.com", mockEmail.LastTo)
		assert.Equal(t, "MPP Password Reset Verification Code", mockEmail.LastSubject)
	})

	t.Run("ForgotPassword_UserNotFound", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/send-code", strings.NewReader(`{"email":"nonexistent@example.com", "scene":"forgot_password"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		require.NoError(t, handler.SendCode(e.NewContext(req, rec)))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestRegisterWithVerification(t *testing.T) {
	db := setupHandlerTestDB(t)
	rdb := setupMiniRedis(t)
	handler := NewAuthHandler(db, rdb, &email.MockEmailService{}, []byte("test-secret"))
	e := echo.New()

	email := "test@example.com"
	code := "123456"
	storeVerificationCode(t, rdb, "register", email, code)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"testuser", "email":"test@example.com", "password":"Password1234", "code":"123456"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	require.NoError(t, handler.Register(e.NewContext(req, rec)))
	assert.Equal(t, http.StatusCreated, rec.Code)

	// Code should be deleted after use
	exists, _ := rdb.Exists(context.Background(), fmt.Sprintf("auth:code:register:%s", email)).Result()
	assert.Equal(t, int64(0), exists)
}

func TestResetPassword(t *testing.T) {
	db := setupHandlerTestDB(t)
	rdb := setupMiniRedis(t)
	handler := NewAuthHandler(db, rdb, &email.MockEmailService{}, []byte("test-secret"))
	e := echo.New()

	userEmail := "user@example.com"
	createTestUser(t, db, "user", userEmail, "OldPassword123")

	t.Run("Success", func(t *testing.T) {
		code := "654321"
		storeVerificationCode(t, rdb, "forgot_password", userEmail, code)

		req := httptest.NewRequest(http.MethodPost, "/api/auth/reset-password", strings.NewReader(`{"email":"user@example.com", "code":"654321", "password":"NewPassword1234"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()

		require.NoError(t, handler.ResetPassword(e.NewContext(req, rec)))
		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify password updated
		var user models.User
		require.NoError(t, db.First(&user, "email = ?", userEmail).Error)
		err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("NewPassword1234"))
		assert.NoError(t, err)
	})

	t.Run("InvalidCode", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/reset-password", strings.NewReader(`{"email":"user@example.com", "code":"000000", "password":"AnotherPassword1234"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()

		require.NoError(t, handler.ResetPassword(e.NewContext(req, rec)))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("InvalidCodeLockoutDeletesCode", func(t *testing.T) {
		code := "111111"
		storeVerificationCode(t, rdb, "forgot_password", userEmail, code)

		for range 5 {
			req := httptest.NewRequest(http.MethodPost, "/api/auth/reset-password", strings.NewReader(`{"email":"user@example.com", "code":"000000", "password":"LockedPassword1234"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			require.NoError(t, handler.ResetPassword(e.NewContext(req, rec)))
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		}

		exists, err := rdb.Exists(context.Background(), "auth:code:forgot_password:user@example.com").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(0), exists)

		req := httptest.NewRequest(http.MethodPost, "/api/auth/reset-password", strings.NewReader(`{"email":"user@example.com", "code":"111111", "password":"LockedPassword1234"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		require.NoError(t, handler.ResetPassword(e.NewContext(req, rec)))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("RequiresRedis", func(t *testing.T) {
		handlerWithoutRedis := NewAuthHandler(db, nil, &email.MockEmailService{}, []byte("test-secret"))
		req := httptest.NewRequest(http.MethodPost, "/api/auth/reset-password", strings.NewReader(`{"email":"user@example.com", "code":"123456", "password":"AnotherPassword1234"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()

		require.NoError(t, handlerWithoutRedis.ResetPassword(e.NewContext(req, rec)))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})
}

func TestRegisterSuccess(t *testing.T) {
	db := setupHandlerTestDB(t)
	rdb := setupMiniRedis(t)
	handler := NewAuthHandler(db, rdb, &email.MockEmailService{}, []byte("test-secret"))
	storeVerificationCode(t, rdb, "register", "new@example.com", "123456")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"newuser", "email":"new@example.com", "password":"Password1234", "code":"123456"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	require.NoError(t, handler.Register(e.NewContext(req, rec)))

	assert.Equal(t, http.StatusCreated, rec.Code)
	var response AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.NotEmpty(t, response.Token)
	assert.Equal(t, "newuser", response.Username)

	var user models.User
	require.NoError(t, db.First(&user, "username = ?", "newuser").Error)
	assert.NotEmpty(t, user.PasswordHash)
	assert.Equal(t, "new@example.com", user.Email)
}

func TestRegisterUsernameExists(t *testing.T) {
	db := setupHandlerTestDB(t)
	createTestUser(t, db, "existing", "existing@example.com", "Password1234")
	rdb := setupMiniRedis(t)
	handler := NewAuthHandler(db, rdb, &email.MockEmailService{}, []byte("test-secret"))
	storeVerificationCode(t, rdb, "register", "new@example.com", "123456")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"existing", "email":"new@example.com", "password":"Password1234", "code":"123456"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	require.NoError(t, handler.Register(e.NewContext(req, rec)))
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestRegisterRequiresRedis(t *testing.T) {
	db := setupHandlerTestDB(t)
	handler := NewAuthHandler(db, nil, &email.MockEmailService{}, []byte("test-secret"))

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"newuser", "email":"new@example.com", "password":"Password1234", "code":"123456"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	require.NoError(t, handler.Register(e.NewContext(req, rec)))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestRegisterWeakPassword(t *testing.T) {
	db := setupHandlerTestDB(t)
	handler := NewAuthHandler(db, nil, &email.MockEmailService{}, []byte("test-secret"))

	tests := []struct {
		name     string
		password string
	}{
		{"TooShort", "Pass123"},
		{"NoUppercase", "password123"},
		{"NoLowercase", "PASSWORD123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"newuser", "email":"new@example.com", "password":"`+tt.password+`", "code":"123456"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()

			require.NoError(t, handler.Register(e.NewContext(req, rec)))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestLoginSuccess(t *testing.T) {
	db := setupHandlerTestDB(t)
	createTestUser(t, db, "user1", "user1@example.com", "Password1234")
	handler := NewAuthHandler(db, nil, &email.MockEmailService{}, []byte("test-secret"))
	handler.SetUsernameLoginEnabled(true)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"user1", "password":"Password1234"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	require.NoError(t, handler.Login(e.NewContext(req, rec)))

	require.Equal(t, http.StatusOK, rec.Code)
	var response AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.NotEmpty(t, response.Token)
	assert.Equal(t, "user1", response.Username)
}

func TestLoginInvalidPassword(t *testing.T) {
	db := setupHandlerTestDB(t)
	createTestUser(t, db, "user1", "user1@example.com", "Password1234")
	handler := NewAuthHandler(db, nil, &email.MockEmailService{}, []byte("test-secret"))
	handler.SetUsernameLoginEnabled(true)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"user1", "password":"wrongpassword"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	require.NoError(t, handler.Login(e.NewContext(req, rec)))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestLoginUserNotFound(t *testing.T) {
	db := setupHandlerTestDB(t)
	handler := NewAuthHandler(db, nil, &email.MockEmailService{}, []byte("test-secret"))
	handler.SetUsernameLoginEnabled(true)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"nonexistent", "password":"Password1234"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	require.NoError(t, handler.Login(e.NewContext(req, rec)))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

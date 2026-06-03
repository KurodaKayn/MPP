package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func createTestUser(t *testing.T, db *gorm.DB, username, password string) models.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err)

	user := models.User{
		Username:     username,
		PasswordHash: string(hash),
		Role:         "user",
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func TestRegisterSuccess(t *testing.T) {
	db := setupHandlerTestDB(t)
	handler := NewAuthHandler(db, []byte("test-secret"))

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"newuser", "password":"Password1234"}`))
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
}

func TestRegisterUsernameExists(t *testing.T) {
	db := setupHandlerTestDB(t)
	createTestUser(t, db, "existing", "Password1234")
	handler := NewAuthHandler(db, []byte("test-secret"))

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"existing", "password":"Password1234"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	require.NoError(t, handler.Register(e.NewContext(req, rec)))
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestRegisterWeakPassword(t *testing.T) {
	db := setupHandlerTestDB(t)
	handler := NewAuthHandler(db, []byte("test-secret"))

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
			req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"newuser", "password":"`+tt.password+`"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()

			require.NoError(t, handler.Register(e.NewContext(req, rec)))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestLoginSuccess(t *testing.T) {
	db := setupHandlerTestDB(t)
	createTestUser(t, db, "user1", "Password1234")
	handler := NewAuthHandler(db, []byte("test-secret"))
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
	createTestUser(t, db, "user1", "Password1234")
	handler := NewAuthHandler(db, []byte("test-secret"))
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
	handler := NewAuthHandler(db, []byte("test-secret"))
	handler.SetUsernameLoginEnabled(true)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"nonexistent", "password":"Password1234"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	require.NoError(t, handler.Login(e.NewContext(req, rec)))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

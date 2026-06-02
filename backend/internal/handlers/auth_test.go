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
)

func TestLoginRejectsUsernameLoginWhenDisabled(t *testing.T) {
	db := setupHandlerTestDB(t)
	handler := NewAuthHandler(db, []byte("test-secret"))

	rec := performLoginRequest(t, handler, `{"username":"intruder"}`)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	var count int64
	require.NoError(t, db.Model(&models.User{}).Where("username = ?", "intruder").Count(&count).Error)
	assert.Zero(t, count)
}

func TestLoginRejectsExistingUserWhenUsernameLoginDisabled(t *testing.T) {
	db := setupHandlerTestDB(t)
	require.NoError(t, db.Create(&models.User{Username: "owner", Role: "user"}).Error)
	handler := NewAuthHandler(db, []byte("test-secret"))

	rec := performLoginRequest(t, handler, `{"username":"owner"}`)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestLoginAutoCreatesUserWhenUsernameLoginEnabled(t *testing.T) {
	db := setupHandlerTestDB(t)
	handler := NewAuthHandler(db, []byte("test-secret"))
	handler.SetUsernameLoginEnabled(true)

	rec := performLoginRequest(t, handler, `{"username":"creator"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	var response AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.NotEmpty(t, response.Token)
	assert.Equal(t, "creator", response.Username)

	var user models.User
	require.NoError(t, db.First(&user, "username = ?", "creator").Error)
	assert.Equal(t, "user", user.Role)
}

func performLoginRequest(t *testing.T, handler *AuthHandler, body string) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	require.NoError(t, handler.Login(e.NewContext(req, rec)))
	return rec
}

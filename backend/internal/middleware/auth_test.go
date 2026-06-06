package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestGetUserIDFromContextRejectsMissingUser(t *testing.T) {
	e := echo.New()
	c := e.NewContext(nil, nil)

	userID, err := GetUserIDFromContext(c)

	require.Error(t, err)
	require.Equal(t, uuid.Nil, userID)
	require.Contains(t, err.Error(), "user context not found")
}

func TestGetUserIDFromContextRejectsInvalidTokenFormat(t *testing.T) {
	e := echo.New()
	c := e.NewContext(nil, nil)
	c.Set("user", "not a token")

	userID, err := GetUserIDFromContext(c)

	require.Error(t, err)
	require.Equal(t, uuid.Nil, userID)
	require.Contains(t, err.Error(), "invalid jwt token format")
}

func TestGetUserIDFromContextRejectsInvalidClaimsFormat(t *testing.T) {
	e := echo.New()
	c := e.NewContext(nil, nil)
	c.Set("user", jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": uuid.NewString(),
	}))

	userID, err := GetUserIDFromContext(c)

	require.Error(t, err)
	require.Equal(t, uuid.Nil, userID)
	require.Contains(t, err.Error(), "invalid jwt claims format")
}

func TestGetUserIDFromContextReturnsClaimUserID(t *testing.T) {
	e := echo.New()
	c := e.NewContext(nil, nil)
	expectedUserID := uuid.New()
	c.Set("user", jwt.NewWithClaims(jwt.SigningMethodHS256, &JWTCustomClaims{
		UserID:   expectedUserID,
		TenantID: "tenant-acme",
		Role:     "user",
	}))

	userID, err := GetUserIDFromContext(c)

	require.NoError(t, err)
	require.Equal(t, expectedUserID, userID)
}

func TestRequireAdminRejectsMissingUser(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	handler := RequireAdmin()(func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	err := handler(c)

	require.Error(t, err)
	var httpErr *echo.HTTPError
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusUnauthorized, httpErr.Code)
}

func TestRequireAdminRejectsNonAdminRole(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", jwt.NewWithClaims(jwt.SigningMethodHS256, &JWTCustomClaims{
		UserID: uuid.New(),
		Role:   "user",
	}))
	handler := RequireAdmin()(func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	err := handler(c)

	require.Error(t, err)
	var httpErr *echo.HTTPError
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusForbidden, httpErr.Code)
}

func TestRequireAdminAllowsAdminRole(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", jwt.NewWithClaims(jwt.SigningMethodHS256, &JWTCustomClaims{
		UserID: uuid.New(),
		Role:   " ADMIN ",
	}))
	handler := RequireAdmin()(func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	err := handler(c)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
}

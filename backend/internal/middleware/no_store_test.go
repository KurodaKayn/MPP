package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestNoStoreAPIResponsesSetsPrivateNoStoreHeaders(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/user/dashboard/stats", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := NoStoreAPIResponses()(func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
	})

	require.NoError(t, handler(c))
	require.Equal(t, NoStoreCacheControl, rec.Header().Get(echo.HeaderCacheControl))
	require.Equal(t, "no-cache", rec.Header().Get("Pragma"))
	require.Equal(t, "0", rec.Header().Get("Expires"))
}

func TestNoStoreAPIResponsesSkipsNonAPIPaths(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := NoStoreAPIResponses()(func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"message": "pong"})
	})

	require.NoError(t, handler(c))
	require.Empty(t, rec.Header().Get(echo.HeaderCacheControl))
}

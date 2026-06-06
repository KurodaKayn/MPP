package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
)

func TestStickyWriterAddsCookieContext(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	until := now.Add(5 * time.Second)
	e := echo.New()
	middleware := StickyWriterWithConfig(StickyWriterConfig{Now: func() time.Time {
		return now
	}})
	handler := middleware(func(c echo.Context) error {
		got, ok := dbrouter.StickyWriterUntil(c.Request().Context())
		require.True(t, ok)
		require.True(t, got.Equal(until))
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/user/dashboard/projects", nil)
	req.AddCookie(stickyWriterRequestCookie(strconv.FormatInt(until.UnixMilli(), 10)))
	rec := httptest.NewRecorder()

	require.NoError(t, handler(e.NewContext(req, rec)))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, rec.Header().Values("Set-Cookie"))
}

func TestStickyWriterIgnoresExpiredCookie(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	e := echo.New()
	middleware := StickyWriterWithConfig(StickyWriterConfig{Now: func() time.Time {
		return now
	}})
	handler := middleware(func(c echo.Context) error {
		_, ok := dbrouter.StickyWriterUntil(c.Request().Context())
		require.False(t, ok)
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/user/dashboard/projects", nil)
	req.AddCookie(stickyWriterRequestCookie(strconv.FormatInt(now.Add(-time.Second).UnixMilli(), 10)))
	rec := httptest.NewRecorder()

	require.NoError(t, handler(e.NewContext(req, rec)))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestStickyWriterIgnoresCookieBeyondMaxTTL(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	e := echo.New()
	middleware := StickyWriterWithConfig(StickyWriterConfig{Now: func() time.Time {
		return now
	}})
	handler := middleware(func(c echo.Context) error {
		_, ok := dbrouter.StickyWriterUntil(c.Request().Context())
		require.False(t, ok)
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/user/dashboard/projects", nil)
	req.AddCookie(stickyWriterRequestCookie(strconv.FormatInt(now.Add(time.Hour).UnixMilli(), 10)))
	rec := httptest.NewRecorder()

	require.NoError(t, handler(e.NewContext(req, rec)))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestStickyWriterRefreshesCookieAfterSuccessfulMutation(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	completed := now.Add(250 * time.Millisecond)
	clockCalls := 0
	e := echo.New()
	middleware := StickyWriterWithConfig(StickyWriterConfig{
		TTL:  5 * time.Second,
		Path: "/api/user",
		Now: func() time.Time {
			clockCalls++
			if clockCalls == 1 {
				return now
			}
			return completed
		},
	})
	handler := middleware(func(c echo.Context) error {
		return c.NoContent(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/user/dashboard/projects", strings.NewReader("{}"))
	rec := httptest.NewRecorder()

	require.NoError(t, handler(e.NewContext(req, rec)))
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, strconv.FormatInt(completed.Add(5*time.Second).UnixMilli(), 10), rec.Header().Get(StickyWriterHeader))

	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, StickyWriterCookieName, cookies[0].Name)
	require.Equal(t, strconv.FormatInt(completed.Add(5*time.Second).UnixMilli(), 10), cookies[0].Value)
	require.Equal(t, "/api/user", cookies[0].Path)
	require.True(t, cookies[0].HttpOnly)
	require.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)
	require.Equal(t, 5, cookies[0].MaxAge)
}

func TestStickyWriterCapsRefreshTTL(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	e := echo.New()
	middleware := StickyWriterWithConfig(StickyWriterConfig{
		TTL:  time.Hour,
		Now:  func() time.Time { return now },
		Path: "/api",
	})
	handler := middleware(func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPatch, "/api/user/dashboard/projects/1", nil)
	rec := httptest.NewRecorder()

	require.NoError(t, handler(e.NewContext(req, rec)))
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, 30, cookies[0].MaxAge)
	require.Equal(t, strconv.FormatInt(now.Add(30*time.Second).UnixMilli(), 10), cookies[0].Value)
}

func TestStickyWriterDoesNotRefreshAfterRead(t *testing.T) {
	e := echo.New()
	handler := StickyWriterWithConfig(StickyWriterConfig{})(func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/user/dashboard/projects", nil)
	rec := httptest.NewRecorder()

	require.NoError(t, handler(e.NewContext(req, rec)))
	require.Empty(t, rec.Header().Get(StickyWriterHeader))
	require.Empty(t, rec.Header().Values("Set-Cookie"))
}

func TestStickyWriterDoesNotRefreshAfterFailedMutation(t *testing.T) {
	e := echo.New()
	handler := StickyWriterWithConfig(StickyWriterConfig{})(func(c echo.Context) error {
		return c.NoContent(http.StatusBadRequest)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/user/dashboard/projects", nil)
	rec := httptest.NewRecorder()

	require.NoError(t, handler(e.NewContext(req, rec)))
	require.Empty(t, rec.Header().Get(StickyWriterHeader))
	require.Empty(t, rec.Header().Values("Set-Cookie"))
}

func TestStickyWriterDoesNotRefreshOutsideConfiguredPath(t *testing.T) {
	e := echo.New()
	handler := StickyWriterWithConfig(StickyWriterConfig{Path: "/api"})(func(c echo.Context) error {
		return c.NoContent(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodPost, "/internal/media/resolve", nil)
	rec := httptest.NewRecorder()

	require.NoError(t, handler(e.NewContext(req, rec)))
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Empty(t, rec.Header().Get(StickyWriterHeader))
	require.Empty(t, rec.Header().Values("Set-Cookie"))
}

func TestStickyWriterDoesNotRefreshWhenHandlerReturnsError(t *testing.T) {
	e := echo.New()
	handler := StickyWriterWithConfig(StickyWriterConfig{})(func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusInternalServerError, "boom")
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/user/dashboard/projects/1", nil)
	rec := httptest.NewRecorder()

	err := handler(e.NewContext(req, rec))
	require.Error(t, err)
	require.Empty(t, rec.Header().Get(StickyWriterHeader))
	require.Empty(t, rec.Header().Values("Set-Cookie"))
}

func TestStickyWriterRejectsInvalidCookieValue(t *testing.T) {
	e := echo.New()
	handler := StickyWriterWithConfig(StickyWriterConfig{})(func(c echo.Context) error {
		_, ok := dbrouter.StickyWriterUntil(c.Request().Context())
		require.False(t, ok)
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/user/dashboard/projects", nil)
	req.AddCookie(stickyWriterRequestCookie("not-a-timestamp"))
	rec := httptest.NewRecorder()

	require.NoError(t, handler(e.NewContext(req, rec)))
	require.Equal(t, http.StatusOK, rec.Code)
}

func stickyWriterRequestCookie(value string) *http.Cookie {
	return &http.Cookie{
		Name:     StickyWriterCookieName,
		Value:    value,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	}
}

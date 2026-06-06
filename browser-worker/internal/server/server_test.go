package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-browser-worker/internal/session"
)

func TestInternalBrowserSessionRoutesRequireBearerToken(t *testing.T) {
	server := &Server{
		sessions:      session.NewManagerWithLimit(0),
		internalToken: "test-worker-token",
	}
	e := echo.New()
	server.RegisterRoutes(e)

	for _, tc := range []struct {
		name          string
		authorization string
		wantStatus    int
	}{
		{name: "missing token", wantStatus: http.StatusUnauthorized},
		{name: "wrong token", authorization: "Bearer wrong-token", wantStatus: http.StatusUnauthorized},
		{name: "valid token", authorization: "Bearer test-worker-token", wantStatus: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/internal/browser-sessions/missing", nil)
			if tc.authorization != "" {
				req.Header.Set(echo.HeaderAuthorization, tc.authorization)
			}
			rec := httptest.NewRecorder()

			e.ServeHTTP(rec, req)

			require.Equal(t, tc.wantStatus, rec.Code)
		})
	}
}

func TestInternalBrowserSessionRoutesFailClosedWhenTokenMissing(t *testing.T) {
	server := &Server{
		sessions: session.NewManagerWithLimit(0),
	}
	e := echo.New()
	server.RegisterRoutes(e)
	req := httptest.NewRequest(http.MethodGet, "/internal/browser-sessions/missing", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer any-token")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	browserruntime "github.com/kurodakayn/mpp-browser-worker/internal/runtime"
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

func TestRuntimeReferenceResponseKeepsDriverNeutralRuntimeIdentity(t *testing.T) {
	response := runtimeReferenceResponse(browserruntime.SessionReference{
		Driver:    browserruntime.DriverKubernetes,
		RuntimeID: "pod-123",
		CDPEndpoint: browserruntime.Endpoint{
			Host: "10.42.0.7",
			Port: 9222,
		},
		StreamEndpoint: browserruntime.Endpoint{
			Host: "10.42.0.7",
			Port: 6080,
		},
		CleanupLabels: map[string]string{"session_id": "session-123"},
	})

	assert.Equal(t, browserruntime.DriverKubernetes, response.Driver)
	assert.Equal(t, "pod-123", response.RuntimeID)
	assert.Equal(t, "10.42.0.7", response.CdpEndpoint.Host)
	assert.Equal(t, 9222, response.CdpEndpoint.Port)
	require.NotNil(t, response.CleanupLabels)
	assert.Equal(t, "session-123", (*response.CleanupLabels)["session_id"])
}

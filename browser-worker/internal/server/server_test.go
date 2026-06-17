package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workerpublish "github.com/kurodakayn/mpp-browser-worker/internal/publish"
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

func TestStartDouyinPublishRequiresDouyinSession(t *testing.T) {
	server := &Server{
		sessions:      session.NewManagerWithLimit(0),
		internalToken: "test-worker-token",
	}
	server.sessions.Put(&session.WorkerSession{
		ID:       "session-1",
		Platform: "zhihu",
	})
	e := echo.New()
	server.RegisterRoutes(e)
	req := httptest.NewRequest(
		http.MethodPost,
		"/internal/browser-sessions/session-1/publish/douyin",
		bytes.NewBufferString(`{"title":"Title","content":"Body","cover_image_base64":"aGVsbG8="}`),
	)
	req.Header.Set(echo.HeaderAuthorization, "Bearer test-worker-token")
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "session platform is not douyin")
}

func TestStartDouyinPublishStartsRunnerWithRequestPayload(t *testing.T) {
	type runnerCall struct {
		req       workerpublish.DouyinDraftRequest
		sessionID string
	}
	started := make(chan runnerCall, 1)
	previousRunner := runDouyinDraft
	runDouyinDraft = func(_ context.Context, workerSession *session.WorkerSession, req workerpublish.DouyinDraftRequest) error {
		started <- runnerCall{req: req, sessionID: workerSession.ID}
		return nil
	}
	t.Cleanup(func() {
		runDouyinDraft = previousRunner
	})

	server := &Server{
		sessions:      session.NewManagerWithLimit(0),
		internalToken: "test-worker-token",
	}
	server.sessions.Put(&session.WorkerSession{
		ID:       "session-1",
		Platform: "douyin",
	})
	e := echo.New()
	server.RegisterRoutes(e)
	body := workerpublish.DouyinDraftRequest{
		Title:            "Draft title",
		Content:          "Draft body",
		CoverImageBase64: "aGVsbG8=",
		CoverImageName:   "cover.png",
	}
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(
		http.MethodPost,
		"/internal/browser-sessions/session-1/publish/douyin",
		bytes.NewReader(bodyBytes),
	)
	req.Header.Set(echo.HeaderAuthorization, "Bearer test-worker-token")
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Contains(t, rec.Body.String(), "douyin publish script started")
	select {
	case got := <-started:
		assert.Equal(t, "session-1", got.sessionID)
		assert.Equal(t, body, got.req)
	case <-time.After(time.Second):
		t.Fatal("expected Douyin publish runner to start")
	}
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

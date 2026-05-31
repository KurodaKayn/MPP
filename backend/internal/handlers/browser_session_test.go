package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/handlers"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupBrowserSessionHandlerTest(t *testing.T) (*echo.Echo, *handlers.BrowserSessionHandler, *gorm.DB) {
	e := echo.New()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&models.User{},
		&models.PlatformAccount{},
		&models.RemoteBrowserSession{},
	)
	require.NoError(t, err)

	worker := publisher.NewMockBrowserWorkerClient()
	t.Cleanup(func() {
		require.NoError(t, worker.Close())
	})
	store := publisher.NewCookieStore(db)
	svc := services.NewBrowserSessionService(db, worker, store)
	h := handlers.NewBrowserSessionHandler(svc)

	return e, h, db
}

func setHandlerUser(c echo.Context, userID uuid.UUID) {
	c.Set("user", jwt.NewWithClaims(jwt.SigningMethodHS256, &middleware.JWTCustomClaims{
		UserID: userID,
		Role:   "user",
	}))
}

func TestBrowserSessionHandler_StartSession(t *testing.T) {
	e, h, _ := setupBrowserSessionHandlerTest(t)
	userID := uuid.New()
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	req := httptest.NewRequest(http.MethodPost, "/api/user/dashboard/settings/platforms/douyin/browser-session", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("platform")
	c.SetParamValues("douyin")
	setHandlerUser(c, userID)

	require.NoError(t, h.StartSession(c))
	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp dto.StartBrowserSessionResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ready", resp.Status)
}

func TestBrowserSessionHandler_FullFlow(t *testing.T) {
	e, h, _ := setupBrowserSessionHandlerTest(t)
	userID := uuid.New()
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	// 1. Start
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("platform")
	c.SetParamValues("douyin")
	setHandlerUser(c, userID)
	require.NoError(t, h.StartSession(c))

	var startResp dto.StartBrowserSessionResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &startResp))
	sessionID := startResp.SessionID

	// 2. Get
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(sessionID.String())
	setHandlerUser(c, userID)
	require.NoError(t, h.GetSession(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 3. Complete
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(sessionID.String())
	setHandlerUser(c, userID)
	require.NoError(t, h.CompleteSession(c))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestBrowserSessionHandler_CancelSessionReturnsStatus(t *testing.T) {
	e, h, _ := setupBrowserSessionHandlerTest(t)
	userID := uuid.New()
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("platform")
	c.SetParamValues("douyin")
	setHandlerUser(c, userID)
	require.NoError(t, h.StartSession(c))

	var startResp dto.StartBrowserSessionResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &startResp))

	req = httptest.NewRequest(http.MethodDelete, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(startResp.SessionID.String())
	setHandlerUser(c, userID)
	require.NoError(t, h.CancelSession(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	var cancelResp dto.CancelBrowserSessionResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cancelResp))
	assert.Equal(t, startResp.SessionID, cancelResp.SessionID)
	assert.Equal(t, models.BrowserSessionStatusExpired, cancelResp.Status)
}

func TestBrowserSessionHandler_StreamSessionUsesMockStream(t *testing.T) {
	e, h, _ := setupBrowserSessionHandlerTest(t)
	userID := uuid.New()
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("platform")
	c.SetParamValues("douyin")
	setHandlerUser(c, userID)
	require.NoError(t, h.StartSession(c))

	var startResp dto.StartBrowserSessionResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &startResp))
	streamURL, err := url.Parse(startResp.StreamURL)
	require.NoError(t, err)
	streamPathParts := strings.SplitN(streamURL.Path, startResp.SessionID.String()+"/", 2)
	require.Len(t, streamPathParts, 2)
	streamWildcard := streamPathParts[1]

	req = httptest.NewRequest(http.MethodGet, startResp.StreamURL, nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id", "*")
	c.SetParamValues(startResp.SessionID.String(), streamWildcard)
	setHandlerUser(c, userID)

	require.NoError(t, h.StreamSession(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Mock remote browser session")
}

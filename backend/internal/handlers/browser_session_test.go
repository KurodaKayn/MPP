package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/app"
	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/handlers"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestServerStickyWriterCookieRoutesAdminStatsToWriter(t *testing.T) {
	signingKey := []byte("test-secret")
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	router := dbrouter.NewRouter(writer, dbrouter.WithReader(reader))
	seedAdminStatsRouteDatabase(t, writer, "writer", 1, models.PublicationStatusSucceeded)
	seedAdminStatsRouteDatabase(t, reader, "reader", 2, models.PublicationStatusFailed)

	server, err := newServer(serverConfig{
		runtimeConfig: app.RuntimeConfig{
			ProcessRole: app.ProcessRoleAPI,
		},
		jwtSigningKey: signingKey,
		ready:         &atomic.Bool{},
		sqlDB:         writer,
		dbRouter:      router,
	}, serverHandlers{
		adminDashboard: handlers.NewDashboardHandler(services.NewDashboardServiceWithRouter(writer, router)),
	})
	require.NoError(t, err)
	stickyCookie := issueStickyWriterCookie(t, server, "/api/test-sticky-cookie")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/stats", nil)
	req.Header.Set("Authorization", "Bearer "+signTestAdminRouteJWT(t, signingKey, middleware.RoleAdmin))
	req.AddCookie(stickyCookie)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{
		"total_users": 1,
		"total_projects": 1,
		"total_published_publications": 1,
		"total_failed_publications": 0
	}`, rec.Body.String())
}

func TestServerRejectsForgedStickyWriterCookie(t *testing.T) {
	signingKey := []byte("test-secret")
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	router := dbrouter.NewRouter(writer, dbrouter.WithReader(reader))
	seedAdminStatsRouteDatabase(t, writer, "writer", 1, models.PublicationStatusSucceeded)
	seedAdminStatsRouteDatabase(t, reader, "reader", 2, models.PublicationStatusFailed)

	server, err := newServer(serverConfig{
		runtimeConfig: app.RuntimeConfig{
			ProcessRole: app.ProcessRoleAPI,
		},
		jwtSigningKey: signingKey,
		ready:         &atomic.Bool{},
		sqlDB:         writer,
		dbRouter:      router,
	}, serverHandlers{
		adminDashboard: handlers.NewDashboardHandler(services.NewDashboardServiceWithRouter(writer, router)),
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/stats", nil)
	req.Header.Set("Authorization", "Bearer "+signTestAdminRouteJWT(t, signingKey, middleware.RoleAdmin))
	req.AddCookie(stickyWriterAPITestCookie(strconv.FormatInt(time.Now().Add(30*time.Second).UnixMilli(), 10)))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{
		"total_users": 1,
		"total_projects": 2,
		"total_published_publications": 0,
		"total_failed_publications": 2
	}`, rec.Body.String())
}

func TestServerEventualAdminStatsStillUseReaderWithoutStickyCookie(t *testing.T) {
	signingKey := []byte("test-secret")
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	router := dbrouter.NewRouter(writer, dbrouter.WithReader(reader))
	seedAdminStatsRouteDatabase(t, writer, "writer", 1, models.PublicationStatusSucceeded)
	seedAdminStatsRouteDatabase(t, reader, "reader", 2, models.PublicationStatusFailed)

	server, err := newServer(serverConfig{
		runtimeConfig: app.RuntimeConfig{
			ProcessRole: app.ProcessRoleAPI,
		},
		jwtSigningKey: signingKey,
		ready:         &atomic.Bool{},
		sqlDB:         writer,
		dbRouter:      router,
	}, serverHandlers{
		adminDashboard: handlers.NewDashboardHandler(services.NewDashboardServiceWithRouter(writer, router)),
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/stats", nil)
	req.Header.Set("Authorization", "Bearer "+signTestAdminRouteJWT(t, signingKey, middleware.RoleAdmin))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{
		"total_users": 1,
		"total_projects": 2,
		"total_published_publications": 0,
		"total_failed_publications": 2
	}`, rec.Body.String())
}

func TestServerStickyWriterCookieIsSecureOutsideLocalDevelopment(t *testing.T) {
	t.Setenv(app.AppEnvEnv, "production")
	t.Setenv(app.NodeEnvFallbackEnv, "development")

	server, err := newServer(serverConfig{
		runtimeConfig: app.RuntimeConfig{
			ProcessRole: app.ProcessRoleWorker,
		},
		jwtSigningKey: []byte("test-secret"),
		ready:         &atomic.Bool{},
	}, serverHandlers{})
	require.NoError(t, err)

	cookie := issueStickyWriterCookie(t, server, "/api/test-secure-sticky-cookie")

	require.True(t, cookie.Secure)
	require.True(t, cookie.HttpOnly)
	require.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
}

func stickyWriterAPITestCookie(value string) *http.Cookie {
	return &http.Cookie{
		Name:     middleware.StickyWriterCookieName,
		Value:    value,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	}
}

func issueStickyWriterCookie(t *testing.T, server *echo.Echo, path string) *http.Cookie {
	t.Helper()

	server.POST(path, func(c echo.Context) error {
		return c.NoContent(http.StatusCreated)
	})
	req := httptest.NewRequest(http.MethodPost, path, nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, middleware.StickyWriterCookieName, cookies[0].Name)
	return cookies[0]
}

func seedAdminStatsRouteDatabase(t *testing.T, dbName *gorm.DB, prefix string, projects int, publicationStatus string) {
	t.Helper()

	user := models.User{
		ID:           uuid.New(),
		Username:     prefix + "-user",
		Email:        prefix + "-user@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, dbName.Create(&user).Error)
	for idx := range projects {
		project := models.Project{
			ID:            uuid.New(),
			UserID:        user.ID,
			Title:         prefix + "-project-" + strconv.Itoa(idx+1),
			SourceContent: "content",
			Status:        models.ProjectStatusReady,
		}
		require.NoError(t, dbName.Create(&project).Error)
		require.NoError(t, dbName.Create(&models.ProjectPlatformPublication{
			ID:        uuid.New(),
			ProjectID: project.ID,
			Platform:  "wechat",
			Status:    publicationStatus,
		}).Error)
	}
}

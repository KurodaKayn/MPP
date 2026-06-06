package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/app"
	"github.com/kurodakayn/mpp-backend/internal/handlers"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestAdminDashboardRoutesRequireAuthentication(t *testing.T) {
	server := newAdminRouteTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/stats", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAdminDashboardRoutesRejectNonAdminJWT(t *testing.T) {
	signingKey := []byte("test-secret")
	server := newAdminRouteTestServer(t, signingKey...)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/stats", nil)
	req.Header.Set("Authorization", "Bearer "+signTestAdminRouteJWT(t, signingKey, "user"))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestAdminDashboardRoutesAllowAdminJWT(t *testing.T) {
	signingKey := []byte("test-secret")
	server := newAdminRouteTestServer(t, signingKey...)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/stats", nil)
	req.Header.Set("Authorization", "Bearer "+signTestAdminRouteJWT(t, signingKey, middleware.RoleAdmin))
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

func newAdminRouteTestServer(t *testing.T, signingKey ...byte) http.Handler {
	t.Helper()

	key := []byte("test-secret")
	if len(signingKey) > 0 {
		key = signingKey
	}

	db := testsupport.SetupTestDB()
	userID := uuid.New()
	projectID := uuid.New()
	require.NoError(t, db.Create(&models.User{
		ID:           userID,
		Username:     "admin-route-user",
		Email:        "admin-route-user@example.com",
		PasswordHash: "hash",
		Role:         "user",
	}).Error)
	require.NoError(t, db.Create(&models.Project{
		ID:            projectID,
		UserID:        userID,
		Title:         "Admin route project",
		SourceContent: "<p>Hello</p>",
		Status:        models.ProjectStatusDraft,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ID:        uuid.New(),
		ProjectID: projectID,
		Platform:  "wechat",
		Status:    models.PublicationStatusPublished,
	}).Error)

	server, err := newServer(serverConfig{
		runtimeConfig: app.RuntimeConfig{
			ProcessRole: app.ProcessRoleAPI,
		},
		jwtSigningKey: key,
		ready:         &atomic.Bool{},
		sqlDB:         db,
	}, serverHandlers{
		adminDashboard: handlers.NewDashboardHandler(services.NewDashboardService(db)),
	})
	require.NoError(t, err)
	return server
}

func signTestAdminRouteJWT(t *testing.T, signingKey []byte, role string) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &middleware.JWTCustomClaims{
		UserID: uuid.New(),
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	signed, err := token.SignedString(signingKey)
	require.NoError(t, err)
	return signed
}

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/dto"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/middleware"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/models"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/services"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error)

	require.NoError(t, db.Exec(`CREATE TABLE projects (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		title TEXT NOT NULL,
		source_content TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error)

	require.NoError(t, db.Exec(`CREATE TABLE platform_accounts (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		name TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'untested',
		credentials TEXT NOT NULL DEFAULT '{}',
		metadata TEXT NOT NULL DEFAULT '{}',
		last_tested_at DATETIME,
		last_test_error TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error)

	require.NoError(t, db.Exec(`CREATE TABLE project_platform_publications (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		enabled BOOLEAN NOT NULL DEFAULT 1,
		status TEXT NOT NULL,
		config TEXT NOT NULL DEFAULT '{}',
		adapted_content TEXT NOT NULL DEFAULT '{}',
		remote_id TEXT,
		publish_url TEXT,
		error_message TEXT,
		retry_count INTEGER NOT NULL DEFAULT 0,
		last_attempt_at DATETIME,
		published_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error)

	return db
}

func newHandlerTestContext(e *echo.Echo, method, target string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func setContextUser(c echo.Context, userID uuid.UUID) {
	c.Set("user", jwt.NewWithClaims(jwt.SigningMethodHS256, &middleware.JWTCustomClaims{
		UserID: userID,
		Role:   "user",
	}))
}

func TestDashboardHandlerListProjectsNormalizesPagination(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewDashboardHandler(services.NewDashboardService(db))
	c, rec := newHandlerTestContext(e, http.MethodGet, "/api/admin/dashboard/projects?page=invalid&limit=1000")

	require.NoError(t, handler.ListProjects(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dto.PaginationResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 1, resp.Page)
	require.Equal(t, 100, resp.Limit)
}

func TestDashboardHandlerGetProjectPublicationsRejectsInvalidUUID(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewDashboardHandler(services.NewDashboardService(db))
	c, rec := newHandlerTestContext(e, http.MethodGet, "/api/admin/dashboard/projects/not-a-uuid/publications")
	c.SetParamNames("id")
	c.SetParamValues("not-a-uuid")

	require.NoError(t, handler.GetProjectPublications(c))
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var resp dto.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "invalid_request", resp.Error.Code)
}

func TestDashboardHandlerGetProjectPublicationsReturnsNotFound(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewDashboardHandler(services.NewDashboardService(db))
	missingID := uuid.NewString()
	c, rec := newHandlerTestContext(e, http.MethodGet, "/api/admin/dashboard/projects/"+missingID+"/publications")
	c.SetParamNames("id")
	c.SetParamValues(missingID)

	require.NoError(t, handler.GetProjectPublications(c))
	require.Equal(t, http.StatusNotFound, rec.Code)

	var resp dto.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "not_found", resp.Error.Code)
}

func TestUserDashboardHandlerRequiresUserContext(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))
	c, rec := newHandlerTestContext(e, http.MethodGet, "/api/user/dashboard/stats")

	require.NoError(t, handler.GetMyStats(c))
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	var resp dto.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "unauthorized", resp.Error.Code)
}

func TestUserDashboardHandlerListProjectsUsesJWTUserScope(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))

	owner := models.User{Username: "owner"}
	other := models.User{Username: "other"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&other).Error)

	ownerProject := models.Project{
		UserID:        owner.ID,
		Title:         "owner project",
		SourceContent: "owner content",
		Status:        models.ProjectStatusDraft,
		CreatedAt:     time.Now(),
	}
	otherProject := models.Project{
		UserID:        other.ID,
		Title:         "other project",
		SourceContent: "other content",
		Status:        models.ProjectStatusDraft,
		CreatedAt:     time.Now().Add(time.Minute),
	}
	require.NoError(t, db.Create(&ownerProject).Error)
	require.NoError(t, db.Create(&otherProject).Error)

	c, rec := newHandlerTestContext(e, http.MethodGet, "/api/user/dashboard/projects?user_id="+other.ID.String()+"&page=bad&limit=1000")
	setContextUser(c, owner.ID)

	require.NoError(t, handler.ListMyProjects(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Items []dto.ProjectListItem `json:"items"`
		Page  int                   `json:"page"`
		Limit int                   `json:"limit"`
		Total int64                 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 1, resp.Page)
	require.Equal(t, 100, resp.Limit)
	require.Equal(t, int64(1), resp.Total)
	require.Len(t, resp.Items, 1)
	require.Equal(t, ownerProject.ID, resp.Items[0].ID)
	require.Equal(t, owner.ID, resp.Items[0].UserID)
}

func TestUserDashboardHandlerCreateProject(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/user/dashboard/projects",
		strings.NewReader(`{"title":"Post title","source_content":"<p>Body</p>","summary":"Body","platforms":["wechat"]}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, user.ID)

	require.NoError(t, handler.CreateProject(c))
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp dto.ProjectListItem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "Post title", resp.Title)
	require.Equal(t, user.ID, resp.UserID)
	require.Len(t, resp.Publications, 1)
	require.Equal(t, "wechat", resp.Publications[0].Platform)
}

func TestUserDashboardHandlerGetProjectPublicationsReturnsForbidden(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))

	owner := models.User{Username: "owner"}
	stranger := models.User{Username: "stranger"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&stranger).Error)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "owner project",
		SourceContent: "owner content",
		Status:        models.ProjectStatusDraft,
	}
	require.NoError(t, db.Create(&project).Error)

	c, rec := newHandlerTestContext(e, http.MethodGet, "/api/user/dashboard/projects/"+project.ID.String()+"/publications")
	c.SetParamNames("id")
	c.SetParamValues(project.ID.String())
	setContextUser(c, stranger.ID)

	require.NoError(t, handler.GetMyProjectPublications(c))
	require.Equal(t, http.StatusForbidden, rec.Code)

	var resp dto.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "forbidden", resp.Error.Code)
}

func TestUserDashboardHandlerSavesWechatAccount(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/user/dashboard/settings/wechat/account",
		strings.NewReader(`{"app_id":"wx-app","app_secret":"wx-secret"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, user.ID)

	require.NoError(t, handler.SaveWechatAccount(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dto.WechatAccountResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "wechat", resp.Platform)
	require.Equal(t, "wx-app", resp.AppID)
	require.True(t, resp.HasAppSecret)
	require.Equal(t, models.PlatformAccountStatusUntested, resp.Status)
}

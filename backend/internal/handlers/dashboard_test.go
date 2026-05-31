package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
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
		username TEXT NOT NULL UNIQUE,
		password TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'user',
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
		username TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'untested',
		credentials TEXT NOT NULL DEFAULT '{}',
		metadata TEXT NOT NULL DEFAULT '{}',
		cookies TEXT NOT NULL DEFAULT '[]',
		config TEXT NOT NULL DEFAULT '{}',
		avatar_url TEXT,
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

type fakeAIContentEditor struct {
	contentReq       dto.AIEditContentRequest
	contentResp      *dto.AIEditContentResponse
	contentStream    *services.AIServiceStream
	prepublishReq    dto.AIEditPrepublishRequest
	prepublishResp   *dto.AIEditPrepublishResponse
	prepublishStream *services.AIServiceStream
	err              error
}

func (f *fakeAIContentEditor) EditContent(ctx context.Context, req dto.AIEditContentRequest) (*dto.AIEditContentResponse, error) {
	f.contentReq = req
	if f.err != nil {
		return nil, f.err
	}
	return f.contentResp, nil
}

func (f *fakeAIContentEditor) StreamEditContent(ctx context.Context, req dto.AIEditContentRequest) (*services.AIServiceStream, error) {
	f.contentReq = req
	if f.err != nil {
		return nil, f.err
	}
	return f.contentStream, nil
}

func (f *fakeAIContentEditor) EditPrepublish(ctx context.Context, req dto.AIEditPrepublishRequest) (*dto.AIEditPrepublishResponse, error) {
	f.prepublishReq = req
	if f.err != nil {
		return nil, f.err
	}
	return f.prepublishResp, nil
}

func (f *fakeAIContentEditor) StreamEditPrepublish(ctx context.Context, req dto.AIEditPrepublishRequest) (*services.AIServiceStream, error) {
	f.prepublishReq = req
	if f.err != nil {
		return nil, f.err
	}
	return f.prepublishStream, nil
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

func TestUserDashboardHandlerGetAndUpdateProject(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)

	project := models.Project{
		UserID:        user.ID,
		Title:         "Draft title",
		SourceContent: "<p>Draft body</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusPublished,
	}).Error)

	getContext, getRecorder := newHandlerTestContext(e, http.MethodGet, "/api/user/dashboard/projects/"+project.ID.String())
	getContext.SetParamNames("id")
	getContext.SetParamValues(project.ID.String())
	setContextUser(getContext, user.ID)

	require.NoError(t, handler.GetMyProject(getContext))
	require.Equal(t, http.StatusOK, getRecorder.Code)

	var detail dto.ProjectDetail
	require.NoError(t, json.Unmarshal(getRecorder.Body.Bytes(), &detail))
	require.Equal(t, project.ID, detail.ID)
	require.Equal(t, "<p>Draft body</p>", detail.SourceContent)

	updateRequest := httptest.NewRequest(
		http.MethodPut,
		"/api/user/dashboard/projects/"+project.ID.String(),
		strings.NewReader(`{"title":"Updated title","source_content":"<p>Updated body</p>","summary":"Updated","platforms":["zhihu"]}`),
	)
	updateRequest.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	updateRecorder := httptest.NewRecorder()
	updateContext := e.NewContext(updateRequest, updateRecorder)
	updateContext.SetParamNames("id")
	updateContext.SetParamValues(project.ID.String())
	setContextUser(updateContext, user.ID)

	require.NoError(t, handler.UpdateProject(updateContext))
	require.Equal(t, http.StatusOK, updateRecorder.Code)

	require.NoError(t, json.Unmarshal(updateRecorder.Body.Bytes(), &detail))
	require.Equal(t, "Updated title", detail.Title)
	require.Equal(t, "<p>Updated body</p>", detail.SourceContent)
	require.Len(t, detail.Publications, 2)
}

func TestUserDashboardHandlerSaveProjectContentPreservesPrepublishDraft(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)

	project := models.Project{
		UserID:        user.ID,
		Title:         "Draft title",
		SourceContent: "<p>Draft body</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "zhihu",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: []byte(`{"format":"markdown","markdown":"AI draft"}`),
	}).Error)

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/user/dashboard/projects/"+project.ID.String()+"/content",
		strings.NewReader(`{"title":"Updated title","source_content":"<p>Updated body</p>","summary":"Updated"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(project.ID.String())
	setContextUser(c, user.ID)

	require.NoError(t, handler.SaveProjectContent(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var detail dto.ProjectDetail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &detail))
	require.Equal(t, "Updated title", detail.Title)
	require.Equal(t, "<p>Updated body</p>", detail.SourceContent)

	var publication models.ProjectPlatformPublication
	require.NoError(t, db.First(&publication, "project_id = ? AND platform = ?", project.ID, "zhihu").Error)
	require.Equal(t, models.PublicationStatusAdapted, publication.Status)
	require.JSONEq(t, `{"format":"markdown","markdown":"AI draft"}`, string(publication.AdaptedContent))
}

func TestUserDashboardHandlerSaveProjectPlatformsPreservesSelectedDrafts(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)

	project := models.Project{
		UserID:        user.ID,
		Title:         "Draft title",
		SourceContent: "<p>Draft body</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: []byte(`{"format":"html","html":"Wechat draft"}`),
	}).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "zhihu",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: []byte(`{"format":"markdown","markdown":"Zhihu AI draft"}`),
	}).Error)

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/user/dashboard/projects/"+project.ID.String()+"/platforms",
		strings.NewReader(`{"platforms":["zhihu"]}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(project.ID.String())
	setContextUser(c, user.ID)

	require.NoError(t, handler.SaveProjectPlatforms(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var wechat models.ProjectPlatformPublication
	require.NoError(t, db.First(&wechat, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	require.False(t, wechat.Enabled)
	require.Equal(t, models.PublicationStatusDisabled, wechat.Status)

	var zhihu models.ProjectPlatformPublication
	require.NoError(t, db.First(&zhihu, "project_id = ? AND platform = ?", project.ID, "zhihu").Error)
	require.True(t, zhihu.Enabled)
	require.Equal(t, models.PublicationStatusAdapted, zhihu.Status)
	require.JSONEq(t, `{"format":"markdown","markdown":"Zhihu AI draft"}`, string(zhihu.AdaptedContent))
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

func TestUserDashboardHandlerSyncProjectPrepublish(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)

	project := models.Project{
		UserID:        user.ID,
		Title:         "Sync title",
		SourceContent: "<p>Hello <strong>sync</strong></p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "zhihu",
		Enabled:   true,
		Status:    models.PublicationStatusPending,
		Config:    []byte(`{"title":"Sync title"}`),
	}).Error)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/user/dashboard/projects/"+project.ID.String()+"/prepublish/sync",
		strings.NewReader(`{"platforms":["zhihu"],"actor":{"type":"system"}}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(project.ID.String())
	setContextUser(c, user.ID)

	require.NoError(t, handler.SyncProjectPrepublish(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dto.ProjectPublicationsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, project.ID, resp.ProjectID)
	require.Len(t, resp.Items, 1)
	require.Equal(t, "zhihu", resp.Items[0].Platform)
	require.Equal(t, models.PublicationStatusAdapted, resp.Items[0].Status)
	require.Equal(t, "markdown", resp.Items[0].AdaptedContent["format"])
	require.Contains(t, resp.Items[0].AdaptedContent["markdown"], "**sync**")
}

func TestUserDashboardHandlerUpdateProjectPrepublishDraft(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)

	project := models.Project{
		UserID:        user.ID,
		Title:         "Draft title",
		SourceContent: "<p>Draft</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "zhihu",
		Enabled:        true,
		Status:         models.PublicationStatusPublished,
		AdaptedContent: []byte(`{"format":"markdown","markdown":"# Old"}`),
		RemoteID:       "remote-id",
		PublishURL:     "https://example.com/post",
		RetryCount:     3,
	}).Error)

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/user/dashboard/projects/"+project.ID.String()+"/prepublish/zhihu",
		strings.NewReader(`{"adapted_content":{"format":"markdown","markdown":"## Updated"}}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "platform")
	c.SetParamValues(project.ID.String(), "zhihu")
	setContextUser(c, user.ID)

	require.NoError(t, handler.UpdateProjectPrepublishDraft(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dto.ProjectPublicationsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	require.Equal(t, models.PublicationStatusAdapted, resp.Items[0].Status)
	require.Equal(t, "## Updated", resp.Items[0].AdaptedContent["markdown"])
	require.Empty(t, resp.Items[0].PublishURL)
	require.Empty(t, resp.Items[0].RemoteID)
	require.Zero(t, resp.Items[0].RetryCount)
}

func TestUserDashboardHandlerEditContentWithAI(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))
	aiEditor := &fakeAIContentEditor{
		contentResp: &dto.AIEditContentResponse{
			Channel: "content",
			Content: "<p>Sharper draft</p>",
		},
	}
	handler.UseAIContentEditor(aiEditor)

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/user/dashboard/ai/content/edit",
		strings.NewReader(`{"title":"Draft","content":"<p>Draft</p>","message":"Make it sharper","conversation":[{"role":"user","content":"Keep it short"}]}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, user.ID)

	require.NoError(t, handler.EditContentWithAI(c))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "<p>Draft</p>", aiEditor.contentReq.Content)
	require.Equal(t, "Make it sharper", aiEditor.contentReq.Message)
	require.Len(t, aiEditor.contentReq.Conversation, 1)

	var resp dto.AIEditContentResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "content", resp.Channel)
	require.Equal(t, "<p>Sharper draft</p>", resp.Content)
}

func TestUserDashboardHandlerStreamsContentWithAI(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))
	aiEditor := &fakeAIContentEditor{
		contentStream: &services.AIServiceStream{
			Body:        io.NopCloser(strings.NewReader("streamed markdown")),
			ContentType: "text/markdown; charset=utf-8",
		},
	}
	handler.UseAIContentEditor(aiEditor)

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/user/dashboard/ai/content/edit/stream",
		strings.NewReader(`{"content":"Draft","message":"Edit"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, user.ID)

	require.NoError(t, handler.StreamEditContentWithAI(c))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "text/markdown; charset=utf-8", rec.Header().Get(echo.HeaderContentType))
	require.Equal(t, "streamed markdown", rec.Body.String())
	require.Equal(t, "Draft", aiEditor.contentReq.Content)
	require.Equal(t, "Edit", aiEditor.contentReq.Message)
}

func TestUserDashboardHandlerEditPrepublishWithAI(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))
	aiEditor := &fakeAIContentEditor{
		prepublishResp: &dto.AIEditPrepublishResponse{
			Channel:  "prepublish",
			Platform: "wechat",
			AdaptedContent: map[string]interface{}{
				"format": "html",
				"html":   "<p>Concise draft</p>",
			},
			Content: "<p>Concise draft</p>",
		},
	}
	handler.UseAIContentEditor(aiEditor)

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/user/dashboard/ai/prepublish/edit",
		strings.NewReader(`{"title":"Draft","platform":"wechat","adapted_content":{"format":"html","html":"<p>Long draft</p>"},"message":"Make it concise"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, user.ID)

	require.NoError(t, handler.EditPrepublishWithAI(c))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "wechat", aiEditor.prepublishReq.Platform)
	require.Equal(t, "Make it concise", aiEditor.prepublishReq.Message)
	require.Equal(t, "html", aiEditor.prepublishReq.AdaptedContent["format"])

	var resp dto.AIEditPrepublishResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "prepublish", resp.Channel)
	require.Equal(t, "wechat", resp.Platform)
	require.Equal(t, "<p>Concise draft</p>", resp.Content)
}

func TestUserDashboardHandlerEditContentWithAIRequiresConfiguredEditor(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/user/dashboard/ai/content/edit",
		strings.NewReader(`{"content":"Draft","message":"Edit"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, user.ID)

	require.NoError(t, handler.EditContentWithAI(c))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestUserDashboardHandlerPublishProjectRejectsDisabledPublication(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)

	project := models.Project{
		UserID:        user.ID,
		Title:         "owner project",
		SourceContent: "owner content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Enabled:   false,
		Status:    models.PublicationStatusDisabled,
	}).Error)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/user/dashboard/projects/"+project.ID.String()+"/publish",
		strings.NewReader(`{"platform":"wechat"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(project.ID.String())
	setContextUser(c, user.ID)

	require.NoError(t, handler.PublishProject(c))
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var resp dto.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "invalid_request", resp.Error.Code)
	require.Equal(t, "publication is disabled for this project", resp.Error.Message)
}

func TestUserDashboardHandlerCreatesXManualPublishIntent(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "owner project",
		SourceContent: "owner content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "x",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: []byte(`{"text":"manual x post"}`),
	}).Error)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/user/dashboard/projects/"+project.ID.String()+"/publish",
		strings.NewReader(`{"platform":"x","mode":"manual"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(project.ID.String())
	setContextUser(c, user.ID)

	require.NoError(t, handler.PublishProject(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "manual_required", resp["status"])

	publishURL, ok := resp["publish_url"].(string)
	require.True(t, ok)
	parsed, err := url.Parse(publishURL)
	require.NoError(t, err)
	require.Equal(t, "manual x post", parsed.Query().Get("text"))
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

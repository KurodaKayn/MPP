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
	"github.com/kurodakayn/mpp-backend/internal/contracts"
	dbobs "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/observability"
	"github.com/kurodakayn/mpp-backend/internal/pkg/streamgate"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type noopProjectDocumentInitializer struct{}

func (noopProjectDocumentInitializer) InitializeProjectDocument(context.Context, uuid.UUID) error {
	return nil
}

func (noopProjectDocumentInitializer) SyncProjectSourceContent(context.Context, uuid.UUID) error {
	return nil
}

func setupHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL UNIQUE,
		email TEXT NOT NULL UNIQUE,
		is_email_verified BOOLEAN NOT NULL DEFAULT 0,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'user',
		created_at DATETIME,
		updated_at DATETIME
	)`).Error)

	require.NoError(t, db.Exec(`CREATE TABLE projects (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		workspace_id TEXT,
		collab_document_id TEXT UNIQUE,
		title TEXT NOT NULL,
		source_content TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error)

	require.NoError(t, db.Exec(`CREATE TABLE workspaces (
		id TEXT PRIMARY KEY,
		owner_user_id TEXT NOT NULL,
		name TEXT NOT NULL,
		slug TEXT,
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		deleted_at DATETIME
	)`).Error)

	require.NoError(t, db.Exec(`CREATE TABLE workspace_members (
		workspace_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		role TEXT NOT NULL,
		invited_by TEXT,
		joined_at DATETIME,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (workspace_id, user_id)
	)`).Error)

	require.NoError(t, db.Exec(`CREATE TABLE collab_documents (
		id TEXT PRIMARY KEY,
		owner_user_id TEXT NOT NULL,
		title TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		schema_version INTEGER NOT NULL DEFAULT 1,
		current_seq INTEGER NOT NULL DEFAULT 0,
		last_edited_by TEXT,
		last_edited_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		deleted_at DATETIME
	)`).Error)

	require.NoError(t, db.Exec(`CREATE TABLE project_collaborators (
		project_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		role TEXT NOT NULL,
		created_by TEXT NOT NULL,
		created_at DATETIME,
		PRIMARY KEY (project_id, user_id)
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

	require.NoError(t, db.Exec(`CREATE TABLE extension_callback_tokens (
		id TEXT PRIMARY KEY,
		execution_id TEXT NOT NULL,
		project_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		token TEXT NOT NULL UNIQUE,
		expires_at DATETIME NOT NULL,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error)

	require.NoError(t, db.Exec(`CREATE TABLE extension_execution_events (
		id TEXT PRIMARY KEY,
		callback_token_id TEXT NOT NULL,
		execution_id TEXT NOT NULL,
		project_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		event_id TEXT NOT NULL UNIQUE,
		platform TEXT NOT NULL,
		status TEXT NOT NULL,
		message TEXT,
		remote_id TEXT,
		publish_url TEXT,
		error_message TEXT,
		metadata TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME
	)`).Error)

	return db
}

type handlerFakeProjectDraftCompiler struct{}

func (handlerFakeProjectDraftCompiler) CompileProjectDrafts(ctx context.Context, project *models.Project, publications []models.ProjectPlatformPublication, platforms []string) (map[string][]byte, error) {
	drafts := make(map[string][]byte, len(platforms))
	for _, platform := range platforms {
		switch platform {
		case "zhihu":
			drafts[platform] = []byte(`{"format":"markdown","markdown":"Hello **sync**"}`)
		case "wechat":
			drafts[platform] = []byte(`{"format":"html","html":"<p>Hello <strong>sync</strong></p>"}`)
		case "x", "douyin":
			drafts[platform] = []byte(`{"format":"text","text":"Hello sync"}`)
		default:
			drafts[platform] = []byte(`{"format":"text","text":"Hello sync"}`)
		}
	}
	return drafts, nil
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

type recordingTraceObserver struct {
	traceIDs []string
}

func (r *recordingTraceObserver) ObserveQuery(ctx context.Context, _ dbobs.QueryObservation) {
	r.traceIDs = append(r.traceIDs, observability.TraceIDFromContext(ctx))
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

func TestDashboardHandlerPropagatesTraceContextToDatabaseQueries(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	observer := &recordingTraceObserver{}
	require.NoError(t, dbobs.InstallQueryObserver(db, observer))
	handler := NewDashboardHandler(services.NewDashboardService(db))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/stats", nil)
	req = req.WithContext(observability.ContextWithTraceID(req.Context(), "trace-123"))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, handler.GetStats(c))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, observer.traceIDs)
	require.Contains(t, observer.traceIDs, "trace-123")
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

func TestUserDashboardHandlerGetExtensionSessionReturnsCurrentUser(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))
	user := models.User{Username: "creator", Email: "creator@example.com"}
	require.NoError(t, db.Create(&user).Error)

	c, rec := newHandlerTestContext(e, http.MethodGet, "/api/user/dashboard/extension/session")
	setContextUser(c, user.ID)

	require.NoError(t, handler.GetExtensionSession(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dto.ExtensionSessionResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Authenticated)
	require.Equal(t, user.ID, resp.User.ID)
	require.Equal(t, "creator", resp.User.Username)
}

func TestUserDashboardHandlerGetExtensionSessionRequiresUserContext(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))
	c, rec := newHandlerTestContext(e, http.MethodGet, "/api/user/dashboard/extension/session")

	require.NoError(t, handler.GetExtensionSession(c))
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	var resp dto.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "unauthorized", resp.Error.Code)
}

func TestUserDashboardHandlerListExtensionPrepublishReturnsCurrentUserItems(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Douyin draft",
		SourceContent: "source",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "douyin",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: []byte(`{"text":"douyin preview"}`),
	}).Error)

	c, rec := newHandlerTestContext(e, http.MethodGet, "/api/user/dashboard/extension/prepublish")
	setContextUser(c, user.ID)

	require.NoError(t, handler.ListExtensionPrepublish(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dto.ExtensionPrepublishResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	require.Equal(t, project.ID, resp.Items[0].ProjectID)
	require.Equal(t, "douyin preview", resp.Items[0].Platforms[0].Preview)
}

func TestUserDashboardHandlerListExtensionPrepublishRequiresUserContext(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))
	c, rec := newHandlerTestContext(e, http.MethodGet, "/api/user/dashboard/extension/prepublish")

	require.NoError(t, handler.ListExtensionPrepublish(c))
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	var resp dto.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "unauthorized", resp.Error.Code)
}

func TestUserDashboardHandlerCreateExtensionHandoffReturnsHandoff(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Douyin draft",
		SourceContent: "source",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "douyin",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: []byte(`{"format":"text","text":"douyin body"}`),
	}).Error)

	body := `{"project_id":"` + project.ID.String() + `","platforms":["douyin"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/user/dashboard/extension/handoffs", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, user.ID)

	require.NoError(t, handler.CreateExtensionHandoff(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dto.ExtensionPublishHandoff
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, project.ID, resp.Project.ID)
	require.Len(t, resp.Platforms, 1)
	require.Equal(t, "douyin body", resp.Platforms[0].AdaptedContent["text"])
	require.Equal(t, "http://example.com/api/user/dashboard/extension/events", resp.Platforms[0].Callback.URL)
}

func TestUserDashboardHandlerCreateExtensionHandoffRequiresUserContext(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))
	req := httptest.NewRequest(http.MethodPost, "/api/user/dashboard/extension/handoffs", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, handler.CreateExtensionHandoff(c))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestUserDashboardHandlerRecordExtensionEventAcceptsCallbackTokenWithoutUserContext(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	service := services.NewDashboardService(db)
	handler := NewUserDashboardHandler(service)
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Douyin draft",
		SourceContent: "source",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "douyin",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: []byte(`{"format":"text","text":"douyin body"}`),
	}).Error)
	handoff, err := service.CreateExtensionHandoff(user.ID, dto.CreateExtensionHandoffRequest{
		ProjectID: project.ID,
		Platforms: []string{"douyin"},
	}, "http://example.com/api/user/dashboard/extension/events")
	require.NoError(t, err)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/user/dashboard/extension/events",
		strings.NewReader(`{"token":"`+handoff.Platforms[0].Callback.Token+`","event_id":"event-1","platform":"douyin","status":"user_review","message":"Draft prepared","metadata":{"source":"extension"}}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, handler.RecordExtensionEvent(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dto.ExtensionEventCallbackResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.False(t, resp.Duplicate)
}

func TestUserDashboardHandlerRecordExtensionEventRejectsInvalidToken(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/user/dashboard/extension/events",
		strings.NewReader(`{"token":"missing-token","event_id":"event-1","platform":"douyin","status":"failed"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, handler.RecordExtensionEvent(c))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestUserDashboardHandlerListProjectsUsesJWTUserScope(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	other := models.User{Username: "other", Email: "other@example.com"}
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
	require.Equal(t, models.ProjectRoleOwner, resp.Items[0].Role)
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
	require.Equal(t, models.ProjectRoleOwner, resp.Role)
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
	require.Equal(t, models.ProjectRoleOwner, detail.Role)
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

func TestUserDashboardHandlerProjectCollaborators(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	collaborator := models.User{Username: "collaborator", Email: "collaborator@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&collaborator).Error)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Shared title",
		SourceContent: "Shared body",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)

	addReq := httptest.NewRequest(
		http.MethodPost,
		"/api/user/dashboard/projects/"+project.ID.String()+"/collaborators",
		strings.NewReader(`{"email":"collaborator@example.com","role":"editor"}`),
	)
	addReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	addRec := httptest.NewRecorder()
	addContext := e.NewContext(addReq, addRec)
	addContext.SetParamNames("id")
	addContext.SetParamValues(project.ID.String())
	setContextUser(addContext, owner.ID)

	require.NoError(t, handler.AddProjectCollaborator(addContext))
	require.Equal(t, http.StatusCreated, addRec.Code)

	var added dto.ProjectCollaborator
	require.NoError(t, json.Unmarshal(addRec.Body.Bytes(), &added))
	require.Equal(t, collaborator.ID, added.UserID)
	require.Equal(t, models.ProjectRoleEditor, added.Role)

	listContext, listRec := newHandlerTestContext(e, http.MethodGet, "/api/user/dashboard/projects/"+project.ID.String()+"/collaborators")
	listContext.SetParamNames("id")
	listContext.SetParamValues(project.ID.String())
	setContextUser(listContext, owner.ID)

	require.NoError(t, handler.ListProjectCollaborators(listContext))
	require.Equal(t, http.StatusOK, listRec.Code)

	var list dto.ProjectCollaboratorsResponse
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &list))
	require.Len(t, list.Items, 1)
	require.Equal(t, collaborator.ID, list.Items[0].UserID)

	getContext, getRecorder := newHandlerTestContext(e, http.MethodGet, "/api/user/dashboard/projects/"+project.ID.String())
	getContext.SetParamNames("id")
	getContext.SetParamValues(project.ID.String())
	setContextUser(getContext, collaborator.ID)

	require.NoError(t, handler.GetMyProject(getContext))
	require.Equal(t, http.StatusOK, getRecorder.Code)

	var detail dto.ProjectDetail
	require.NoError(t, json.Unmarshal(getRecorder.Body.Bytes(), &detail))
	require.Equal(t, project.ID, detail.ID)
	require.Equal(t, models.ProjectRoleEditor, detail.Role)
}

func TestUserDashboardHandlerWorkspaces(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))

	owner := models.User{Username: "workspace-owner", Email: "workspace-owner@example.com"}
	member := models.User{Username: "workspace-member", Email: "workspace-member@example.com"}
	stranger := models.User{Username: "workspace-stranger", Email: "workspace-stranger@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&stranger).Error)

	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/workspaces",
		strings.NewReader(`{"name":"Team Workspace","slug":"team-workspace"}`),
	)
	createReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	createRec := httptest.NewRecorder()
	createContext := e.NewContext(createReq, createRec)
	setContextUser(createContext, owner.ID)

	require.NoError(t, handler.CreateWorkspace(createContext))
	require.Equal(t, http.StatusCreated, createRec.Code)

	var created dto.Workspace
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &created))
	require.Equal(t, owner.ID, created.OwnerUserID)
	require.Equal(t, "Team Workspace", created.Name)
	require.Equal(t, "team-workspace", created.Slug)
	require.Equal(t, models.WorkspaceRoleOwner, created.Role)

	addReq := httptest.NewRequest(
		http.MethodPost,
		"/api/workspaces/"+created.ID.String()+"/members",
		strings.NewReader(`{"email":"workspace-member@example.com","role":"member"}`),
	)
	addReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	addRec := httptest.NewRecorder()
	addContext := e.NewContext(addReq, addRec)
	addContext.SetParamNames("id")
	addContext.SetParamValues(created.ID.String())
	setContextUser(addContext, owner.ID)

	require.NoError(t, handler.AddWorkspaceMember(addContext))
	require.Equal(t, http.StatusCreated, addRec.Code)

	var added dto.WorkspaceMember
	require.NoError(t, json.Unmarshal(addRec.Body.Bytes(), &added))
	require.Equal(t, member.ID, added.UserID)
	require.Equal(t, models.WorkspaceRoleMember, added.Role)

	listContext, listRec := newHandlerTestContext(e, http.MethodGet, "/api/workspaces")
	setContextUser(listContext, member.ID)

	require.NoError(t, handler.ListWorkspaces(listContext))
	require.Equal(t, http.StatusOK, listRec.Code)

	var workspaces dto.WorkspacesResponse
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &workspaces))
	require.Len(t, workspaces.Items, 1)
	require.Equal(t, created.ID, workspaces.Items[0].ID)
	require.Equal(t, models.WorkspaceRoleMember, workspaces.Items[0].Role)

	membersContext, membersRec := newHandlerTestContext(e, http.MethodGet, "/api/workspaces/"+created.ID.String()+"/members")
	membersContext.SetParamNames("id")
	membersContext.SetParamValues(created.ID.String())
	setContextUser(membersContext, owner.ID)

	require.NoError(t, handler.ListWorkspaceMembers(membersContext))
	require.Equal(t, http.StatusOK, membersRec.Code)

	var members dto.WorkspaceMembersResponse
	require.NoError(t, json.Unmarshal(membersRec.Body.Bytes(), &members))
	require.Len(t, members.Items, 2)
	require.Equal(t, owner.ID, members.Items[0].UserID)
	require.Equal(t, member.ID, members.Items[1].UserID)

	updateReq := httptest.NewRequest(
		http.MethodPatch,
		"/api/workspaces/"+created.ID.String(),
		strings.NewReader(`{"name":"Renamed Workspace","slug":"renamed-workspace"}`),
	)
	updateReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	updateRec := httptest.NewRecorder()
	updateContext := e.NewContext(updateReq, updateRec)
	updateContext.SetParamNames("id")
	updateContext.SetParamValues(created.ID.String())
	setContextUser(updateContext, owner.ID)

	require.NoError(t, handler.UpdateWorkspace(updateContext))
	require.Equal(t, http.StatusOK, updateRec.Code)

	var updated dto.Workspace
	require.NoError(t, json.Unmarshal(updateRec.Body.Bytes(), &updated))
	require.Equal(t, "Renamed Workspace", updated.Name)
	require.Equal(t, "renamed-workspace", updated.Slug)

	updateMemberReq := httptest.NewRequest(
		http.MethodPatch,
		"/api/workspaces/"+created.ID.String()+"/members/"+member.ID.String(),
		strings.NewReader(`{"role":"viewer"}`),
	)
	updateMemberReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	updateMemberRec := httptest.NewRecorder()
	updateMemberContext := e.NewContext(updateMemberReq, updateMemberRec)
	updateMemberContext.SetParamNames("id", "userId")
	updateMemberContext.SetParamValues(created.ID.String(), member.ID.String())
	setContextUser(updateMemberContext, owner.ID)

	require.NoError(t, handler.UpdateWorkspaceMember(updateMemberContext))
	require.Equal(t, http.StatusOK, updateMemberRec.Code)

	var updatedMember dto.WorkspaceMember
	require.NoError(t, json.Unmarshal(updateMemberRec.Body.Bytes(), &updatedMember))
	require.Equal(t, models.WorkspaceRoleViewer, updatedMember.Role)

	forbiddenReq := httptest.NewRequest(
		http.MethodPost,
		"/api/workspaces/"+created.ID.String()+"/members",
		strings.NewReader(`{"user_id":"`+stranger.ID.String()+`","role":"member"}`),
	)
	forbiddenReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	forbiddenRec := httptest.NewRecorder()
	forbiddenContext := e.NewContext(forbiddenReq, forbiddenRec)
	forbiddenContext.SetParamNames("id")
	forbiddenContext.SetParamValues(created.ID.String())
	setContextUser(forbiddenContext, member.ID)

	require.NoError(t, handler.AddWorkspaceMember(forbiddenContext))
	require.Equal(t, http.StatusForbidden, forbiddenRec.Code)

	removeContext, removeRec := newHandlerTestContext(e, http.MethodDelete, "/api/workspaces/"+created.ID.String()+"/members/"+member.ID.String())
	removeContext.SetParamNames("id", "userId")
	removeContext.SetParamValues(created.ID.String(), member.ID.String())
	setContextUser(removeContext, owner.ID)

	require.NoError(t, handler.RemoveWorkspaceMember(removeContext))
	require.Equal(t, http.StatusNoContent, removeRec.Code)
}

func TestUserDashboardHandlerWorkspaceProjects(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	svc := services.NewDashboardService(db)
	handler := NewUserDashboardHandler(svc)

	owner := models.User{Username: "workspace-project-owner", Email: "workspace-project-owner@example.com"}
	member := models.User{Username: "workspace-project-member", Email: "workspace-project-member@example.com"}
	viewer := models.User{Username: "workspace-project-viewer", Email: "workspace-project-viewer@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&viewer).Error)

	workspace, err := svc.CreateWorkspace(owner.ID, dto.CreateWorkspaceRequest{Name: "Project Workspace"})
	require.NoError(t, err)
	_, err = svc.AddWorkspaceMember(workspace.ID, owner.ID, dto.AddWorkspaceMemberRequest{
		UserID: member.ID,
		Role:   models.WorkspaceRoleMember,
	})
	require.NoError(t, err)
	_, err = svc.AddWorkspaceMember(workspace.ID, owner.ID, dto.AddWorkspaceMemberRequest{
		UserID: viewer.ID,
		Role:   models.WorkspaceRoleViewer,
	})
	require.NoError(t, err)

	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/workspaces/"+workspace.ID.String()+"/projects",
		strings.NewReader(`{"title":"Team Project","source_content":"<p>team</p>","platforms":["wechat"]}`),
	)
	createReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	createRec := httptest.NewRecorder()
	createContext := e.NewContext(createReq, createRec)
	createContext.SetParamNames("id")
	createContext.SetParamValues(workspace.ID.String())
	setContextUser(createContext, member.ID)

	require.NoError(t, handler.CreateWorkspaceProject(createContext))
	require.Equal(t, http.StatusCreated, createRec.Code)

	var created dto.ProjectListItem
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &created))
	require.Equal(t, member.ID, created.UserID)
	require.NotNil(t, created.WorkspaceID)
	require.Equal(t, workspace.ID, *created.WorkspaceID)
	require.Equal(t, models.ProjectRoleOwner, created.Role)

	listContext, listRec := newHandlerTestContext(e, http.MethodGet, "/api/workspaces/"+workspace.ID.String()+"/projects?page=bad&limit=1000")
	listContext.SetParamNames("id")
	listContext.SetParamValues(workspace.ID.String())
	setContextUser(listContext, owner.ID)

	require.NoError(t, handler.ListWorkspaceProjects(listContext))
	require.Equal(t, http.StatusOK, listRec.Code)

	var projects dto.PaginationResponse
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &projects))
	require.Equal(t, int64(1), projects.Total)
	items, ok := projects.Items.([]interface{})
	require.True(t, ok)
	require.Len(t, items, 1)
	item, ok := items[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "Team Project", item["title"])
	require.Equal(t, models.ProjectRoleEditor, item["role"])
	require.Equal(t, workspace.ID.String(), item["workspace_id"])

	forbiddenReq := httptest.NewRequest(
		http.MethodPost,
		"/api/workspaces/"+workspace.ID.String()+"/projects",
		strings.NewReader(`{"title":"Viewer Project","source_content":"<p>viewer</p>","platforms":["wechat"]}`),
	)
	forbiddenReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	forbiddenRec := httptest.NewRecorder()
	forbiddenContext := e.NewContext(forbiddenReq, forbiddenRec)
	forbiddenContext.SetParamNames("id")
	forbiddenContext.SetParamValues(workspace.ID.String())
	setContextUser(forbiddenContext, viewer.ID)

	require.NoError(t, handler.CreateWorkspaceProject(forbiddenContext))
	require.Equal(t, http.StatusForbidden, forbiddenRec.Code)
}

func TestUserDashboardHandlerCreateProjectCollabSession(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	collabService := services.NewCollabDocumentService(db)
	collabService.UseSessionConfig(services.CollabDocumentSessionConfig{
		TokenSecret:      []byte("collab-secret"),
		WebsocketURLBase: "ws://collab.test",
	})
	collabService.UseProjectDocumentInitializer(noopProjectDocumentInitializer{})
	dashboardService := services.NewDashboardService(db)
	dashboardService.SetCollabDocumentService(collabService)
	handler := NewUserDashboardHandler(dashboardService)

	owner := models.User{Username: "collab-owner", Email: "collab-owner@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	project := models.Project{
		UserID:        owner.ID,
		Title:         "Realtime project",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/user/dashboard/projects/"+project.ID.String()+"/collab/session",
		nil,
	)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(project.ID.String())
	setContextUser(c, owner.ID)

	require.NoError(t, handler.CreateProjectCollabSession(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var session contracts.CollabDocumentSession
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &session))
	require.Equal(t, contracts.Editor, session.Role)
	require.Equal(t, "ws://collab.test/collab/documents/"+session.DocumentId.String(), session.WebsocketUrl)
	require.NotEmpty(t, session.Token)

	var savedProject models.Project
	require.NoError(t, db.First(&savedProject, "id = ?", project.ID).Error)
	require.NotNil(t, savedProject.CollabDocumentID)
	require.Equal(t, session.DocumentId, *savedProject.CollabDocumentID)
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

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	stranger := models.User{Username: "stranger", Email: "stranger@example.com"}
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
	service := services.NewDashboardService(db)
	service.SetDraftCompiler(handlerFakeProjectDraftCompiler{})
	handler := NewUserDashboardHandler(service)

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

func TestUserDashboardHandlerSyncProjectPrepublishRejectsActivePublish(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	service := services.NewDashboardService(db)
	service.SetDraftCompiler(handlerFakeProjectDraftCompiler{})
	handler := NewUserDashboardHandler(service)

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
		ProjectID:      project.ID,
		Platform:       "zhihu",
		Enabled:        true,
		Status:         models.PublicationStatusPublishing,
		Config:         []byte(`{"title":"Sync title"}`),
		AdaptedContent: []byte(`{"format":"markdown","markdown":"ready"}`),
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
	require.Equal(t, http.StatusConflict, rec.Code)

	var resp dto.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "publish_in_progress", resp.Error.Code)

	var publication models.ProjectPlatformPublication
	require.NoError(t, db.First(&publication, "project_id = ? AND platform = ?", project.ID, "zhihu").Error)
	require.Equal(t, models.PublicationStatusPublishing, publication.Status)
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

func TestUserDashboardHandlerEditContentWithAIEnforcesUserConcurrency(t *testing.T) {
	e := echo.New()
	db := setupHandlerTestDB(t)
	handler := NewUserDashboardHandler(services.NewDashboardService(db))
	aiEditor := &fakeAIContentEditor{
		contentResp: &dto.AIEditContentResponse{
			Channel: "content",
			Content: "unused",
		},
	}
	handler.UseAIContentEditor(aiEditor)
	limiter := streamgate.New(nil, streamgate.Config{
		Enabled: true,
		AI: streamgate.Limits{
			User: 1,
			TTL:  time.Minute,
		},
	})
	handler.UseStreamLimiter(limiter)

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	lease, err := limiter.Acquire(context.Background(), streamgate.AcquireRequest{
		Kind:     streamgate.KindAI,
		UserID:   user.ID,
		TenantID: middleware.DefaultTenantID,
		IP:       "192.0.2.10",
		Resource: "content",
	})
	require.NoError(t, err)
	defer lease.Release(context.Background())

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/user/dashboard/ai/content/edit",
		strings.NewReader(`{"content":"Draft","message":"Edit"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.RemoteAddr = "192.0.2.10:12345"
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, user.ID)

	require.NoError(t, handler.EditContentWithAI(c))
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Empty(t, aiEditor.contentReq.Message)
}

func TestShouldAcquireBrowserStreamLeaseForLongLivedPaths(t *testing.T) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/stream/token/websockify", nil)
	c := e.NewContext(req, httptest.NewRecorder())
	require.True(t, shouldAcquireBrowserStreamLease(c, false, "websockify"))

	req = httptest.NewRequest(http.MethodGet, "/stream/token/vnc.html?path=api/browser/websockify", nil)
	c = e.NewContext(req, httptest.NewRecorder())
	require.True(t, shouldAcquireBrowserStreamLease(c, false, "vnc.html"))

	req = httptest.NewRequest(http.MethodGet, "/stream/token/vnc.html", nil)
	c = e.NewContext(req, httptest.NewRecorder())
	require.False(t, shouldAcquireBrowserStreamLease(c, false, "vnc.html"))
}

func TestBrowserStreamProxyErrorHandlerMapsTimeoutToGatewayTimeout(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)

	browserStreamProxyErrorHandler(rec, req, context.DeadlineExceeded)

	require.Equal(t, http.StatusGatewayTimeout, rec.Code)
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

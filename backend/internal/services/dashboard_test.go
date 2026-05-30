package services_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	pkgx "github.com/kurodakayn/mpp-backend/internal/pkg/x"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type fakeXOAuth2Provider struct {
	authConfig       pkgx.OAuth2Config
	authState        string
	authChallenge    string
	exchangeCode     string
	exchangeVerifier string
	refreshConfig    pkgx.OAuth2Config
	refreshToken     string
	token            pkgx.OAuth2Token
	user             pkgx.User
}

func (f *fakeXOAuth2Provider) AuthorizationURL(config pkgx.OAuth2Config, state, codeChallenge string) (string, error) {
	f.authConfig = config
	f.authState = state
	f.authChallenge = codeChallenge

	endpoint := url.URL{
		Scheme: "https",
		Host:   "x.example.com",
		Path:   "/i/oauth2/authorize",
	}
	query := endpoint.Query()
	query.Set("state", state)
	query.Set("code_challenge", codeChallenge)
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func (f *fakeXOAuth2Provider) Exchange(ctx context.Context, config pkgx.OAuth2Config, code, codeVerifier string) (pkgx.OAuth2Token, error) {
	f.exchangeCode = code
	f.exchangeVerifier = codeVerifier
	return f.token, nil
}

func (f *fakeXOAuth2Provider) Refresh(ctx context.Context, config pkgx.OAuth2Config, refreshToken string) (pkgx.OAuth2Token, error) {
	f.refreshConfig = config
	f.refreshToken = refreshToken
	return f.token, nil
}

func (f *fakeXOAuth2Provider) Me(ctx context.Context, accessToken string) (pkgx.User, error) {
	return f.user, nil
}

func setupTestDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	db.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	db.Exec(`CREATE TABLE projects (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		title TEXT NOT NULL,
		source_content TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	db.Exec(`CREATE TABLE platform_accounts (
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
	)`)

	db.Exec(`CREATE TABLE project_platform_publications (
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
	)`)

	return db
}

type fakeWechatTester struct {
	result dto.WechatConnectionTestResponse
	appID  string
	secret string
}

func (f *fakeWechatTester) Test(appID, appSecret string) dto.WechatConnectionTestResponse {
	f.appID = appID
	f.secret = appSecret
	return f.result
}

type fakePlatformPublisher struct {
	config datatypes.JSON
}

func (f *fakePlatformPublisher) ValidateConfig(config []byte) error {
	return nil
}

func (f *fakePlatformPublisher) AdaptContent(project *models.Project) ([]byte, error) {
	return nil, nil
}

func (f *fakePlatformPublisher) Publish(ctx context.Context, pub *models.ProjectPlatformPublication, account *models.PlatformAccount) (string, string, error) {
	f.config = append(datatypes.JSON(nil), pub.Config...)
	return "remote-id", "https://example.com/published", nil
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func TestGetStats(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	u1 := models.User{Username: "test1"}
	u2 := models.User{Username: "test2"}
	db.Create(&u1)
	db.Create(&u2)

	p1 := models.Project{UserID: u1.ID, Title: "p1", SourceContent: "c", Status: models.ProjectStatusDraft}
	p2 := models.Project{UserID: u2.ID, Title: "p2", SourceContent: "c", Status: models.ProjectStatusDraft}
	db.Create(&p1)
	db.Create(&p2)

	db.Create(&models.ProjectPlatformPublication{ProjectID: p1.ID, Platform: "wechat", Status: models.PublicationStatusPublished})
	db.Create(&models.ProjectPlatformPublication{ProjectID: p2.ID, Platform: "zhihu", Status: models.PublicationStatusFailed})

	// Test Admin scope (nil scopeUserID)
	stats, err := s.GetStats(nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), stats.TotalUsers)
	assert.Equal(t, int64(2), stats.TotalProjects)
	assert.Equal(t, int64(1), stats.TotalPublishedPublications)
	assert.Equal(t, int64(1), stats.TotalFailedPublications)

	// Test Personal scope (u1)
	statsScoped, errScoped := s.GetStats(&u1.ID)
	assert.NoError(t, errScoped)
	assert.Equal(t, int64(1), statsScoped.TotalUsers)
	assert.Equal(t, int64(1), statsScoped.TotalProjects)
	assert.Equal(t, int64(1), statsScoped.TotalPublishedPublications)
	assert.Equal(t, int64(0), statsScoped.TotalFailedPublications)
}

func TestListProjects(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	u1 := models.User{Username: "test"}
	u2 := models.User{Username: "other"}
	db.Create(&u1)
	db.Create(&u2)

	p1 := models.Project{UserID: u1.ID, Title: "p1", SourceContent: "c1", Status: models.ProjectStatusPublished, CreatedAt: time.Now().Add(-1 * time.Hour)}
	p2 := models.Project{UserID: u1.ID, Title: "p2", SourceContent: "c2", Status: models.ProjectStatusDraft, CreatedAt: time.Now()}
	p3 := models.Project{UserID: u2.ID, Title: "p3", SourceContent: "c3", Status: models.ProjectStatusDraft, CreatedAt: time.Now()}
	db.Create(&p1)
	db.Create(&p2)
	db.Create(&p3)

	db.Create(&models.ProjectPlatformPublication{ProjectID: p1.ID, Platform: "wechat", Status: models.PublicationStatusPublished, PublishURL: "url1"})

	// Test global admin pagination
	res, err := s.ListProjects(1, 10, "", "", "", nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), res.Total)

	// Test Personal scope (u1)
	resScoped, errScoped := s.ListProjects(1, 10, "", "", "", &u1.ID)
	assert.NoError(t, errScoped)
	assert.Equal(t, int64(2), resScoped.Total)
	items := resScoped.Items.([]dto.ProjectListItem)
	assert.Equal(t, 2, len(items))
	// Ensure p3 is not in list
	for _, item := range items {
		assert.NotEqual(t, p3.ID, item.ID)
	}
}

func TestCreateProjectCreatesSelectedPublications(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	db.Create(&user)

	resp, err := s.CreateProject(user.ID, dto.CreateProjectRequest{
		Title:         "WeChat title",
		SourceContent: "<p>Hello WeChat</p>",
		Summary:       "Hello WeChat",
		CoverImageURL: "data:image/png;base64,aGVsbG8=",
		Platforms:     []string{"wechat", "wechat", "bilibili"},
	})

	assert.NoError(t, err)
	assert.Equal(t, "WeChat title", resp.Title)
	assert.Equal(t, models.ProjectStatusReady, resp.Status)
	assert.Len(t, resp.Publications, 2)

	var project models.Project
	assert.NoError(t, db.First(&project, "id = ?", resp.ID).Error)
	assert.Equal(t, user.ID, project.UserID)
	assert.Equal(t, "<p>Hello WeChat</p>", project.SourceContent)

	var wechatPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&wechatPub, "project_id = ? AND platform = ?", resp.ID, "wechat").Error)
	assert.Equal(t, models.PublicationStatusPending, wechatPub.Status)

	var config map[string]string
	assert.NoError(t, json.Unmarshal(wechatPub.Config, &config))
	assert.Equal(t, "WeChat title", config["title"])
	assert.Equal(t, "Hello WeChat", config["digest"])
	assert.Equal(t, "data:image/png;base64,aGVsbG8=", config["cover_image_url"])

	var adapted map[string]string
	assert.NoError(t, json.Unmarshal(wechatPub.AdaptedContent, &adapted))
	assert.Empty(t, adapted)

	var bilibiliPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&bilibiliPub, "project_id = ? AND platform = ?", resp.ID, "bilibili").Error)
	assert.Equal(t, models.PublicationStatusPending, bilibiliPub.Status)
}

func TestCreateProjectRejectsInvalidInput(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	db.Create(&user)

	_, err := s.CreateProject(user.ID, dto.CreateProjectRequest{
		Title:         "Missing platform",
		SourceContent: "content",
	})
	assert.ErrorIs(t, err, services.ErrInvalidProject)

	_, err = s.CreateProject(user.ID, dto.CreateProjectRequest{
		Title:         "Unknown platform",
		SourceContent: "content",
		Platforms:     []string{"threads"},
	})
	assert.ErrorIs(t, err, services.ErrInvalidProject)
}

func TestGetProjectReturnsSourceContentForOwner(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner"}
	stranger := models.User{Username: "stranger"}
	db.Create(&owner)
	db.Create(&stranger)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Existing post",
		SourceContent: "<p>Editable body</p>",
		Status:        models.ProjectStatusReady,
	}
	db.Create(&project)
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusPublished,
	})

	resp, err := s.GetProject(project.ID, &owner.ID)
	assert.NoError(t, err)
	assert.Equal(t, project.ID, resp.ID)
	assert.Equal(t, "<p>Editable body</p>", resp.SourceContent)
	assert.Len(t, resp.Publications, 1)

	_, err = s.GetProject(project.ID, &stranger.ID)
	assert.ErrorIs(t, err, services.ErrForbidden)
}

func TestUpdateProjectRebuildsSelectedPublications(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner"}
	stranger := models.User{Username: "stranger"}
	db.Create(&owner)
	db.Create(&stranger)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Old title",
		SourceContent: "old body",
		Status:        models.ProjectStatusPublished,
	}
	db.Create(&project)
	db.Create(&models.ProjectPlatformPublication{
		ProjectID:    project.ID,
		Platform:     "wechat",
		Enabled:      true,
		Status:       models.PublicationStatusPublished,
		PublishURL:   "https://example.com/old",
		RemoteID:     "old-remote",
		PublishedAt:  ptrTime(time.Now()),
		RetryCount:   2,
		ErrorMessage: "old error",
	})
	db.Create(&models.ProjectPlatformPublication{
		ProjectID:    project.ID,
		Platform:     "zhihu",
		Enabled:      true,
		Status:       models.PublicationStatusFailed,
		ErrorMessage: "failed before",
	})

	resp, err := s.UpdateProject(project.ID, owner.ID, dto.UpdateProjectRequest{
		Title:         "New title",
		SourceContent: "<p>New body</p>",
		Summary:       "New body",
		Platforms:     []string{"zhihu", "bilibili"},
	})

	assert.NoError(t, err)
	assert.Equal(t, "New title", resp.Title)
	assert.Equal(t, "<p>New body</p>", resp.SourceContent)
	assert.Len(t, resp.Publications, 3)

	var saved models.Project
	assert.NoError(t, db.First(&saved, "id = ?", project.ID).Error)
	assert.Equal(t, "New title", saved.Title)
	assert.Equal(t, "<p>New body</p>", saved.SourceContent)
	assert.Equal(t, models.ProjectStatusReady, saved.Status)

	var wechatPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&wechatPub, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	assert.False(t, wechatPub.Enabled)
	assert.Equal(t, models.PublicationStatusDisabled, wechatPub.Status)

	var zhihuPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&zhihuPub, "project_id = ? AND platform = ?", project.ID, "zhihu").Error)
	assert.True(t, zhihuPub.Enabled)
	assert.Equal(t, models.PublicationStatusPending, zhihuPub.Status)
	assert.Empty(t, zhihuPub.ErrorMessage)
	assert.Empty(t, zhihuPub.PublishURL)
	assert.Nil(t, zhihuPub.PublishedAt)

	var bilibiliPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&bilibiliPub, "project_id = ? AND platform = ?", project.ID, "bilibili").Error)
	assert.True(t, bilibiliPub.Enabled)
	assert.Equal(t, models.PublicationStatusPending, bilibiliPub.Status)

	_, err = s.UpdateProject(project.ID, stranger.ID, dto.UpdateProjectRequest{
		Title:         "Not allowed",
		SourceContent: "content",
		Platforms:     []string{"wechat"},
	})
	assert.ErrorIs(t, err, services.ErrForbidden)
}

func TestSyncProjectPrepublishGeneratesPlatformDrafts(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner"}
	db.Create(&owner)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Platform title",
		SourceContent: `<h2>Heading</h2><p>Hello <strong>draft</strong></p>`,
		Status:        models.ProjectStatusReady,
	}
	db.Create(&project)
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusPending,
		Config:    datatypes.JSON(`{"title":"Platform title"}`),
	})
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "zhihu",
		Enabled:   true,
		Status:    models.PublicationStatusPending,
		Config:    datatypes.JSON(`{"title":"Platform title"}`),
	})
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "x",
		Enabled:   true,
		Status:    models.PublicationStatusPending,
		Config:    datatypes.JSON(`{"title":"Platform title"}`),
	})

	resp, err := s.SyncProjectPrepublish(project.ID, owner.ID, dto.SyncPrepublishRequest{
		Platforms: []string{"wechat", "zhihu", "x"},
		Actor:     dto.SyncActor{Type: "system"},
	})

	assert.NoError(t, err)
	assert.Equal(t, project.ID, resp.ProjectID)
	assert.Len(t, resp.Items, 3)

	var wechatPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&wechatPub, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	assert.Equal(t, models.PublicationStatusAdapted, wechatPub.Status)

	var wechatContent map[string]interface{}
	assert.NoError(t, json.Unmarshal(wechatPub.AdaptedContent, &wechatContent))
	assert.Equal(t, "html", wechatContent["format"])
	assert.Equal(t, `<h2>Heading</h2><p>Hello <strong>draft</strong></p>`, wechatContent["html"])

	var zhihuPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&zhihuPub, "project_id = ? AND platform = ?", project.ID, "zhihu").Error)
	assert.Equal(t, models.PublicationStatusAdapted, zhihuPub.Status)

	var zhihuContent map[string]interface{}
	assert.NoError(t, json.Unmarshal(zhihuPub.AdaptedContent, &zhihuContent))
	assert.Equal(t, "markdown", zhihuContent["format"])
	assert.Contains(t, zhihuContent["markdown"], "## Heading")
	assert.Contains(t, zhihuContent["markdown"], "**draft**")

	var xPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&xPub, "project_id = ? AND platform = ?", project.ID, "x").Error)
	assert.Equal(t, models.PublicationStatusAdapted, xPub.Status)

	var xContent map[string]interface{}
	assert.NoError(t, json.Unmarshal(xPub.AdaptedContent, &xContent))
	assert.Equal(t, "text", xContent["format"])
	assert.Contains(t, xContent["text"], "Platform title")
	assert.Contains(t, xContent["text"], "Hello draft")
}

func TestGetProjectPublications(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	u1 := models.User{Username: "owner"}
	u2 := models.User{Username: "stranger"}
	db.Create(&u1)
	db.Create(&u2)

	p := models.Project{UserID: u1.ID, Title: "p1", SourceContent: "c1", Status: models.ProjectStatusPublished}
	db.Create(&p)

	configJSON := `{"title": "safe_title", "secret_token": "hidden"}`
	contentJSON := `{"summary": "safe_summary", "full_text": "huge..."}`

	pub := models.ProjectPlatformPublication{
		ProjectID:      p.ID,
		Platform:       "wechat",
		Status:         models.PublicationStatusPublished,
		Config:         datatypes.JSON(configJSON),
		AdaptedContent: datatypes.JSON(contentJSON),
	}
	db.Create(&pub)

	// Admin can see it
	res, err := s.GetProjectPublications(p.ID, nil, false)
	assert.NoError(t, err)
	assert.Equal(t, p.ID, res.ProjectID)

	// Owner can see it
	resOwner, errOwner := s.GetProjectPublications(p.ID, &u1.ID, false)
	assert.NoError(t, errOwner)
	assert.Equal(t, p.ID, resOwner.ProjectID)

	// Stranger gets Forbidden
	_, errStranger := s.GetProjectPublications(p.ID, &u2.ID, false)
	assert.ErrorIs(t, errStranger, services.ErrForbidden)
}

func TestBatchPublishProject(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	u := models.User{Username: "tester"}
	db.Create(&u)

	p := models.Project{UserID: u.ID, Title: "p", SourceContent: "c", Status: models.ProjectStatusDraft}
	db.Create(&p)

	// Create publications for multiple platforms
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: p.ID,
		Platform:  "wechat",
		Status:    models.PublicationStatusPending,
		Config:    datatypes.JSON(`{"app_id": "test", "app_secret": "test"}`),
	})
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: p.ID,
		Platform:  "zhihu",
		Status:    models.PublicationStatusPending,
	})

	// Test batch publish
	platforms := []string{"wechat", "zhihu"}
	results, err := s.BatchPublishProject(p.ID, platforms, &u.ID)

	assert.NoError(t, err)
	assert.Equal(t, 2, len(results))

	// Check results
	for _, platform := range platforms {
		assert.Contains(t, results, platform)
	}
}

func TestWechatAccountSettingsSaveMasksSecret(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	db.Create(&user)

	resp, err := s.UpsertWechatAccount(user.ID, dto.UpsertWechatAccountRequest{
		AppID:     "wx-app",
		AppSecret: "wx-secret",
	})
	assert.NoError(t, err)
	assert.Equal(t, "wechat", resp.Platform)
	assert.Equal(t, "wx-app", resp.AppID)
	assert.True(t, resp.HasAppSecret)
	assert.Equal(t, models.PlatformAccountStatusUntested, resp.Status)

	saved, err := s.GetWechatAccount(user.ID)
	assert.NoError(t, err)
	assert.Equal(t, "wx-app", saved.AppID)
	assert.True(t, saved.HasAppSecret)
}

func TestWechatAccountTestUsesSavedSecretAndUpdatesStatus(t *testing.T) {
	db := setupTestDB()
	tester := &fakeWechatTester{
		result: dto.WechatConnectionTestResponse{
			Connected: true,
			Status:    models.PlatformAccountStatusConnected,
			Message:   "ok",
			TestedAt:  time.Now(),
		},
	}
	s := services.NewDashboardServiceWithWechatTester(db, tester)

	user := models.User{Username: "owner"}
	db.Create(&user)

	_, err := s.UpsertWechatAccount(user.ID, dto.UpsertWechatAccountRequest{
		AppID:     "wx-app",
		AppSecret: "wx-secret",
	})
	assert.NoError(t, err)

	result, err := s.TestWechatAccount(user.ID, dto.TestWechatAccountRequest{
		AppID: "wx-app",
	})
	assert.NoError(t, err)
	assert.True(t, result.Connected)
	assert.Equal(t, "wx-app", tester.appID)
	assert.Equal(t, "wx-secret", tester.secret)

	saved, err := s.GetWechatAccount(user.ID)
	assert.NoError(t, err)
	assert.Equal(t, models.PlatformAccountStatusConnected, saved.Status)
	assert.Empty(t, saved.LastTestError)
}

func TestWechatAccountTestDoesNotPersistUnsavedCredentialsStatus(t *testing.T) {
	db := setupTestDB()
	testedAt := time.Now()
	tester := &fakeWechatTester{
		result: dto.WechatConnectionTestResponse{
			Connected: false,
			Status:    models.PlatformAccountStatusFailed,
			Message:   "failed",
			TestedAt:  testedAt,
		},
	}
	s := services.NewDashboardServiceWithWechatTester(db, tester)

	user := models.User{Username: "owner"}
	db.Create(&user)

	_, err := s.UpsertWechatAccount(user.ID, dto.UpsertWechatAccountRequest{
		AppID:     "wx-saved",
		AppSecret: "saved-secret",
	})
	assert.NoError(t, err)

	result, err := s.TestWechatAccount(user.ID, dto.TestWechatAccountRequest{
		AppID:     "wx-draft",
		AppSecret: "draft-secret",
	})
	assert.NoError(t, err)
	assert.False(t, result.Connected)
	assert.Equal(t, "wx-draft", tester.appID)
	assert.Equal(t, "draft-secret", tester.secret)

	saved, err := s.GetWechatAccount(user.ID)
	assert.NoError(t, err)
	assert.Equal(t, models.PlatformAccountStatusUntested, saved.Status)
	assert.Nil(t, saved.LastTestedAt)
	assert.Empty(t, saved.LastTestError)
}

func TestXAccountSettingsClearsUsernameAndMetadataWhenCredentialsChange(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	db.Create(&user)

	_, err := s.UpsertXAccount(user.ID, dto.UpsertXAccountRequest{
		APIKey:            "x-old-key",
		APISecret:         "x-old-secret",
		AccessToken:       "x-old-token",
		AccessTokenSecret: "x-old-token-secret",
		Username:          "oldhandle",
	})
	assert.NoError(t, err)

	var account models.PlatformAccount
	assert.NoError(t, db.First(&account, "user_id = ? AND platform = ?", user.ID, "x").Error)
	assert.NoError(t, db.Model(&account).Update("metadata", datatypes.JSON(`{"username":"oldmeta"}`)).Error)

	_, err = s.UpsertXAccount(user.ID, dto.UpsertXAccountRequest{
		APIKey:            "x-new-key",
		APISecret:         "x-new-secret",
		AccessToken:       "x-new-token",
		AccessTokenSecret: "x-new-token-secret",
	})
	assert.NoError(t, err)

	saved, err := s.GetXAccount(user.ID)
	assert.NoError(t, err)
	assert.Empty(t, saved.Username)
	assert.Equal(t, models.PlatformAccountStatusUntested, saved.Status)

	assert.NoError(t, db.First(&account, "user_id = ? AND platform = ?", user.ID, "x").Error)
	var credentials map[string]string
	assert.NoError(t, json.Unmarshal(account.Credentials, &credentials))
	assert.Equal(t, "x-new-key", credentials["api_key"])
	assert.Empty(t, credentials["username"])

	var metadata map[string]string
	assert.NoError(t, json.Unmarshal(account.Metadata, &metadata))
	assert.Empty(t, metadata["username"])
}

func TestXOAuth2FlowStoresConnectedAccount(t *testing.T) {
	t.Setenv("X_OAUTH2_CLIENT_ID", "client-id")
	t.Setenv("X_OAUTH2_CLIENT_SECRET", "client-secret")

	db := setupTestDB()
	expiresAt := time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)
	provider := &fakeXOAuth2Provider{
		token: pkgx.OAuth2Token{
			AccessToken:  "oauth2-access",
			RefreshToken: "oauth2-refresh",
			Scope:        "tweet.read tweet.write users.read offline.access",
			ExpiresAt:    expiresAt,
		},
		user: pkgx.User{
			ID:       "x-user-id",
			Name:     "Creator",
			Username: "creator",
		},
	}
	s := services.NewDashboardServiceWithXOAuth2Provider(db, provider)

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)

	authURL, err := s.StartXOAuth2(user.ID, "https://app.example.com/api/user/dashboard/settings/x/oauth2/callback")
	require.NoError(t, err)
	require.NotEmpty(t, provider.authState)
	require.NotEmpty(t, provider.authChallenge)
	assert.Equal(t, "client-id", provider.authConfig.ClientID)
	assert.Equal(t, "client-secret", provider.authConfig.ClientSecret)

	parsedAuthURL, err := url.Parse(authURL)
	require.NoError(t, err)
	state := parsedAuthURL.Query().Get("state")
	require.NotEmpty(t, state)

	resp, err := s.CompleteXOAuth2(context.Background(), state, "auth-code")
	require.NoError(t, err)

	assert.Equal(t, "auth-code", provider.exchangeCode)
	assert.NotEmpty(t, provider.exchangeVerifier)
	assert.Equal(t, "oauth2", resp.AuthType)
	assert.Equal(t, "creator", resp.Username)
	assert.True(t, resp.HasOAuth2Refresh)
	assert.Equal(t, models.PlatformAccountStatusConnected, resp.Status)
	require.NotNil(t, resp.ExpiresAt)
	assert.Equal(t, expiresAt, *resp.ExpiresAt)

	var account models.PlatformAccount
	require.NoError(t, db.First(&account, "user_id = ? AND platform = ?", user.ID, "x").Error)

	var credentials map[string]string
	require.NoError(t, json.Unmarshal(account.Credentials, &credentials))
	assert.Equal(t, "oauth2", credentials["auth_type"])
	assert.Equal(t, "oauth2-access", credentials["oauth2_access_token"])
	assert.Equal(t, "oauth2-refresh", credentials["oauth2_refresh_token"])

	var metadata map[string]string
	require.NoError(t, json.Unmarshal(account.Metadata, &metadata))
	assert.Equal(t, "creator", metadata["username"])
	assert.Equal(t, "x-user-id", metadata["user_id"])
}

func TestPublishProjectUsesSavedWechatCredentials(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &fakePlatformPublisher{}
	publisher.Factory.Register("wechat", fakePublisher)
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	db.Create(&user)
	project := models.Project{
		UserID:        user.ID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	db.Create(&project)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"app_id":"stale","app_secret":"stale-secret","title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"summary":"ready"}`),
	}
	db.Create(&pub)
	_, err := s.UpsertWechatAccount(user.ID, dto.UpsertWechatAccountRequest{
		AppID:     "wx-saved",
		AppSecret: "saved-secret",
	})
	assert.NoError(t, err)

	result, err := s.PublishProject(project.ID, "wechat", &user.ID)
	assert.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])

	var config map[string]string
	assert.NoError(t, json.Unmarshal(fakePublisher.config, &config))
	assert.Equal(t, "wx-saved", config["app_id"])
	assert.Equal(t, "saved-secret", config["app_secret"])
	assert.Equal(t, "Title", config["title"])
}

func TestPublishProjectAdaptsPendingPublicationBeforePublishing(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &fakePlatformPublisher{}
	publisher.Factory.Register("wechat", fakePublisher)
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "p1",
		SourceContent: "<p>source</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusPending,
		Config:         datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{}`),
	}
	require.NoError(t, db.Create(&pub).Error)

	result, err := s.PublishProject(project.ID, "wechat", &user.ID)

	require.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "id = ?", pub.ID).Error)
	assert.Equal(t, models.PublicationStatusPublished, saved.Status)
	assert.Empty(t, saved.ErrorMessage)
}

func TestPublishProjectUsesSavedXOAuth2Credentials(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &fakePlatformPublisher{}
	publisher.Factory.Register("x", fakePublisher)
	defer publisher.Factory.Register("x", &publisher.XPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "x",
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"api_key":"stale","api_secret":"stale","access_token":"stale","access_token_secret":"stale","title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"text":"ready"}`),
	}
	require.NoError(t, db.Create(&pub).Error)
	require.NoError(t, db.Create(&models.PlatformAccount{
		UserID:   user.ID,
		Platform: "x",
		Username: "X",
		Status:   models.PlatformAccountStatusConnected,
		Credentials: datatypes.JSON(`{
			"auth_type":"oauth2",
			"oauth2_access_token":"oauth2-access",
			"oauth2_refresh_token":"oauth2-refresh",
			"username":"creator"
		}`),
		Metadata: datatypes.JSON(`{"username":"creator"}`),
	}).Error)

	result, err := s.PublishProject(project.ID, "x", &user.ID)
	require.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])

	var config map[string]interface{}
	require.NoError(t, json.Unmarshal(fakePublisher.config, &config))
	assert.Equal(t, "oauth2", config["auth_type"])
	assert.Equal(t, "oauth2-access", config["oauth2_access_token"])
	assert.Equal(t, "oauth2-refresh", config["oauth2_refresh_token"])
	assert.Equal(t, "creator", config["username"])
	assert.NotContains(t, config, "api_key")
	assert.NotContains(t, config, "api_secret")
	assert.NotContains(t, config, "access_token")
	assert.NotContains(t, config, "access_token_secret")
	assert.Equal(t, "Title", config["title"])
}

func TestPublishProjectRefreshesExpiredXOAuth2Token(t *testing.T) {
	t.Setenv("X_OAUTH2_CLIENT_ID", "client-id")
	t.Setenv("X_OAUTH2_CLIENT_SECRET", "client-secret")

	db := setupTestDB()
	refreshedExpiresAt := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	provider := &fakeXOAuth2Provider{
		token: pkgx.OAuth2Token{
			AccessToken:  "new-oauth2-access",
			RefreshToken: "new-oauth2-refresh",
			Scope:        "tweet.read tweet.write users.read offline.access",
			ExpiresAt:    refreshedExpiresAt,
		},
	}
	s := services.NewDashboardServiceWithXOAuth2Provider(db, provider)
	fakePublisher := &fakePlatformPublisher{}
	publisher.Factory.Register("x", fakePublisher)
	defer publisher.Factory.Register("x", &publisher.XPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "x",
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"text":"ready"}`),
	}
	require.NoError(t, db.Create(&pub).Error)

	expiredAt := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	credentials, err := json.Marshal(map[string]interface{}{
		"auth_type":            "oauth2",
		"oauth2_access_token":  "old-oauth2-access",
		"oauth2_refresh_token": "oauth2-refresh",
		"oauth2_expires_at":    expiredAt,
		"username":             "creator",
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.PlatformAccount{
		UserID:      user.ID,
		Platform:    "x",
		Username:    "X",
		Status:      models.PlatformAccountStatusConnected,
		Credentials: datatypes.JSON(credentials),
		Metadata:    datatypes.JSON(`{"username":"creator"}`),
	}).Error)

	result, err := s.PublishProject(project.ID, "x", &user.ID)
	require.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])
	assert.Equal(t, "oauth2-refresh", provider.refreshToken)
	assert.Equal(t, "client-id", provider.refreshConfig.ClientID)
	assert.Equal(t, "client-secret", provider.refreshConfig.ClientSecret)
	assert.Empty(t, provider.refreshConfig.RedirectURI)

	var config map[string]interface{}
	require.NoError(t, json.Unmarshal(fakePublisher.config, &config))
	assert.Equal(t, "oauth2", config["auth_type"])
	assert.Equal(t, "new-oauth2-access", config["oauth2_access_token"])
	assert.Equal(t, "new-oauth2-refresh", config["oauth2_refresh_token"])
	assert.Equal(t, "creator", config["username"])

	var account models.PlatformAccount
	require.NoError(t, db.First(&account, "user_id = ? AND platform = ?", user.ID, "x").Error)
	var savedCredentials map[string]interface{}
	require.NoError(t, json.Unmarshal(account.Credentials, &savedCredentials))
	assert.Equal(t, "new-oauth2-access", savedCredentials["oauth2_access_token"])
	assert.Equal(t, "new-oauth2-refresh", savedCredentials["oauth2_refresh_token"])
	assert.Equal(t, "tweet.read tweet.write users.read offline.access", savedCredentials["oauth2_scope"])
}

func TestCreateXPostIntentReturnsManualPublishURL(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "p1",
		SourceContent: "<p>source content</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "x",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"text":"hello x & \u4e2d\u6587"}`),
	}
	require.NoError(t, db.Create(&pub).Error)

	result, err := s.CreateXPostIntent(project.ID, &user.ID)
	require.NoError(t, err)
	assert.Equal(t, "manual_required", result["status"])
	assert.Equal(t, "x", result["platform"])

	publishURL, ok := result["publish_url"].(string)
	require.True(t, ok)
	parsed, err := url.Parse(publishURL)
	require.NoError(t, err)
	assert.Equal(t, "https", parsed.Scheme)
	assert.Equal(t, "x.com", parsed.Host)
	assert.Equal(t, "/intent/tweet", parsed.Path)
	assert.Equal(t, "hello x & \u4e2d\u6587", parsed.Query().Get("text"))

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "id = ?", pub.ID).Error)
	assert.Equal(t, models.PublicationStatusAdapted, saved.Status)
	assert.Equal(t, publishURL, saved.PublishURL)
	assert.Empty(t, saved.ErrorMessage)
}

func TestCreateXPostIntentAdaptsPendingPublication(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "pending x",
		SourceContent: "<p>source content</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "x",
		Enabled:        true,
		Status:         models.PublicationStatusPending,
		Config:         datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{}`),
	}
	require.NoError(t, db.Create(&pub).Error)

	result, err := s.CreateXPostIntent(project.ID, &user.ID)

	require.NoError(t, err)
	assert.Equal(t, "manual_required", result["status"])

	publishURL, ok := result["publish_url"].(string)
	require.True(t, ok)
	parsed, err := url.Parse(publishURL)
	require.NoError(t, err)
	assert.Contains(t, parsed.Query().Get("text"), "pending x")
	assert.Contains(t, parsed.Query().Get("text"), "source content")

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "id = ?", pub.ID).Error)
	assert.Equal(t, models.PublicationStatusAdapted, saved.Status)
	assert.Contains(t, string(saved.AdaptedContent), `"format":"text"`)
}

func TestPublishProjectRejectsDisabledPublication(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	db.Create(&user)
	project := models.Project{
		UserID:        user.ID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	db.Create(&project)
	db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        false,
		Status:         models.PublicationStatusDisabled,
		Config:         datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"summary":"ready"}`),
	})

	_, err := s.PublishProject(project.ID, "wechat", &user.ID)
	assert.ErrorIs(t, err, services.ErrPublicationDisabled)
}

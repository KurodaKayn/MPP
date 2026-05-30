package services_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/dto"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/models"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/publisher"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/services"
	"github.com/stretchr/testify/assert"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

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
		name TEXT NOT NULL,
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
	res, err := s.GetProjectPublications(p.ID, nil)
	assert.NoError(t, err)
	assert.Equal(t, p.ID, res.ProjectID)

	// Owner can see it
	resOwner, errOwner := s.GetProjectPublications(p.ID, &u1.ID)
	assert.NoError(t, errOwner)
	assert.Equal(t, p.ID, resOwner.ProjectID)

	// Stranger gets Forbidden
	_, errStranger := s.GetProjectPublications(p.ID, &u2.ID)
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

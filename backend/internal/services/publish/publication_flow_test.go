//nolint:gosec // Test fixtures use fake OAuth credential strings.
package publish_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage/fake"
	pkgx "github.com/kurodakayn/mpp-backend/internal/pkg/x"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func createConnectedPublishAccount(t *testing.T, db *gorm.DB, userID uuid.UUID, platform string, credentials datatypes.JSON) models.PlatformAccount {
	t.Helper()
	workspaceID := models.PersonalWorkspaceID(userID)
	if len(credentials) == 0 {
		credentials = datatypes.JSON(`{}`)
	}
	account := models.PlatformAccount{
		UserID:       userID,
		WorkspaceID:  &workspaceID,
		Platform:     platform,
		Username:     platform,
		DisplayName:  platform,
		ShareScope:   models.PlatformAccountSharePrivate,
		Status:       models.PlatformAccountStatusConnected,
		HealthStatus: models.PlatformAccountHealthHealthy,
		Credentials:  credentials,
		Metadata:     datatypes.JSON(`{}`),
		Cookies:      datatypes.JSON(`[]`),
		Config:       datatypes.JSON(`{}`),
	}
	ownerID := userID
	account.OwnerUserID = &ownerID
	account.ConnectedByUserID = &ownerID
	require.NoError(t, db.Create(&account).Error)
	return account
}

func TestBatchPublishProject(t *testing.T) {
	db := testsupport.SetupTestDB()
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

	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Check results
	for _, platform := range platforms {
		assert.Contains(t, results, platform)
	}
}

func TestPublishProjectUsesSavedWechatCredentials(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &testsupport.FakePlatformPublisher{}
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
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.PlatformAccount{}).
		Where("user_id = ? AND platform = ?", user.ID, "wechat").
		Updates(map[string]any{
			"status":        models.PlatformAccountStatusConnected,
			"health_status": models.PlatformAccountHealthHealthy,
		}).Error)

	result, err := s.PublishProject(project.ID, "wechat", &user.ID, uuid.Nil)
	require.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])

	var completedActivity models.ProjectActivity
	require.NoError(t, db.First(
		&completedActivity,
		"project_id = ? AND actor_user_id = ? AND event_type = ?",
		project.ID,
		user.ID,
		models.ProjectActivityPublishCompleted,
	).Error)
	var metadata map[string]string
	require.NoError(t, json.Unmarshal(completedActivity.Metadata, &metadata))
	assert.Equal(t, "wechat", metadata["platform"])
	assert.Equal(t, models.PublicationStatusSucceeded, metadata["status"])
	assert.Equal(t, "remote-id", metadata["remote_id"])

	var queuedActivities int64
	require.NoError(t, db.Model(&models.ProjectActivity{}).
		Where("project_id = ? AND actor_user_id = ? AND event_type = ?", project.ID, user.ID, models.ProjectActivityPublishQueued).
		Count(&queuedActivities).Error)
	assert.Zero(t, queuedActivities)

	var config map[string]string
	require.NoError(t, json.Unmarshal(fakePublisher.Config, &config))
	assert.Equal(t, "wx-saved", config["app_id"])
	assert.Equal(t, "saved-secret", config["app_secret"])
	assert.Equal(t, "Title", config["title"])
}

func TestPublishProjectAllowsEmbeddedWechatCredentialsWithoutSavedAccount(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &testsupport.FakePlatformPublisher{}
	publisher.Factory.Register("wechat", fakePublisher)
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

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
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"app_id":"wx","app_secret":"secret","title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"summary":"ready"}`),
	}
	require.NoError(t, db.Create(&pub).Error)

	result, err := s.PublishProject(project.ID, "wechat", &user.ID, uuid.Nil)

	require.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])
	assert.JSONEq(t, `{"app_id":"wx","app_secret":"secret","title":"Title"}`, string(fakePublisher.Config))
}

func TestPublishProjectPresignsReadyMediaRefsWithoutPersistingSignedURLs(t *testing.T) {
	db := testsupport.SetupTestDB()
	require.NoError(t, db.AutoMigrate(&models.MediaAsset{}))
	s := services.NewDashboardService(db)
	storage := fake.NewClient()
	s.UseObjectStorage(storage, objectstorage.Config{
		Enabled:        true,
		Provider:       objectstorage.ProviderR2,
		Bucket:         "mpp-media",
		UploadURLTTL:   10 * time.Minute,
		DownloadURLTTL: 5 * time.Minute,
	})
	fakePublisher := &testsupport.FakePlatformPublisher{}
	publisher.Factory.Register("wechat", fakePublisher)
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	workspaceID := models.PersonalWorkspaceID(user.ID)
	project := models.Project{
		UserID:        user.ID,
		WorkspaceID:   &workspaceID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	createConnectedPublishAccount(t, db, user.ID, "wechat", datatypes.JSON(`{"app_id":"wx","app_secret":"secret"}`))
	assetID := uuid.New()
	assetProjectID := project.ID
	asset := models.MediaAsset{
		ID:               assetID,
		UserID:           user.ID,
		WorkspaceID:      &workspaceID,
		ProjectID:        &assetProjectID,
		Bucket:           "mpp-media",
		ObjectKey:        "workspaces/" + workspaceID.String() + "/projects/" + project.ID.String() + "/assets/" + assetID.String() + "/hero.png",
		OriginalFilename: "hero.png",
		MimeType:         "image/png",
		SizeBytes:        11,
		Usage:            models.MediaAssetUsageEditorImage,
		Status:           models.MediaAssetStatusReady,
	}
	require.NoError(t, db.Create(&asset).Error)
	mediaRef := "mpp://media/" + assetID.String()
	config, err := json.Marshal(map[string]string{
		"app_id":          "wx",
		"app_secret":      "secret",
		"title":           "Title",
		"cover_image_url": mediaRef,
	})
	require.NoError(t, err)
	adaptedContent, err := json.Marshal(map[string]string{
		"format": "html",
		"html":   `<p><img src="` + mediaRef + `" data-mpp-media-id="` + assetID.String() + `"></p>`,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(config),
		AdaptedContent: datatypes.JSON(adaptedContent),
	}).Error)

	result, err := s.PublishProject(project.ID, "wechat", &user.ID, uuid.Nil)

	require.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])
	expectedURL := "fake://get/mpp-media/" + asset.ObjectKey
	assert.Contains(t, string(fakePublisher.Config), expectedURL)
	assert.NotContains(t, string(fakePublisher.Config), mediaRef)
	assert.Contains(t, string(fakePublisher.AdaptedContent), expectedURL)
	assert.NotContains(t, string(fakePublisher.AdaptedContent), mediaRef)

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	assert.Contains(t, string(saved.Config), mediaRef)
	assert.Contains(t, string(saved.AdaptedContent), mediaRef)
	assert.NotContains(t, string(saved.Config), expectedURL)
	assert.NotContains(t, string(saved.AdaptedContent), expectedURL)
}

func TestPublishProjectPreservesReadyMediaRefsWhenContentPipelineResolverIsConfigured(t *testing.T) {
	t.Setenv("CONTENT_PIPELINE_MEDIA_ENABLED", "true")
	t.Setenv("CONTENT_PIPELINE_MEDIA_RESOLVER_URL", "http://backend:8080/internal/media/resolve")
	t.Setenv("CONTENT_PIPELINE_INTERNAL_TOKEN", "test-internal-token")

	db := testsupport.SetupTestDB()
	require.NoError(t, db.AutoMigrate(&models.MediaAsset{}))
	s := services.NewDashboardService(db)
	storage := fake.NewClient()
	s.UseObjectStorage(storage, objectstorage.Config{
		Enabled:        true,
		Provider:       objectstorage.ProviderR2,
		Bucket:         "mpp-media",
		UploadURLTTL:   10 * time.Minute,
		DownloadURLTTL: 5 * time.Minute,
	})
	fakePublisher := &testsupport.FakePlatformPublisher{}
	publisher.Factory.Register("wechat", fakePublisher)
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	workspaceID := models.PersonalWorkspaceID(user.ID)
	project := models.Project{
		UserID:        user.ID,
		WorkspaceID:   &workspaceID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	assetID := uuid.New()
	assetProjectID := project.ID
	asset := models.MediaAsset{
		ID:               assetID,
		UserID:           user.ID,
		WorkspaceID:      &workspaceID,
		ProjectID:        &assetProjectID,
		Bucket:           "mpp-media",
		ObjectKey:        "workspaces/" + workspaceID.String() + "/projects/" + project.ID.String() + "/assets/" + assetID.String() + "/hero.png",
		OriginalFilename: "hero.png",
		MimeType:         "image/png",
		SizeBytes:        11,
		Usage:            models.MediaAssetUsageEditorImage,
		Status:           models.MediaAssetStatusReady,
	}
	require.NoError(t, db.Create(&asset).Error)
	mediaRef := "mpp://media/" + assetID.String()
	config, err := json.Marshal(map[string]string{
		"app_id":          "wx",
		"app_secret":      "secret",
		"title":           "Title",
		"cover_image_url": mediaRef,
	})
	require.NoError(t, err)
	adaptedContent, err := json.Marshal(map[string]string{
		"format": "html",
		"html":   `<p><img src="` + mediaRef + `" data-mpp-media-id="` + assetID.String() + `"></p>`,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(config),
		AdaptedContent: datatypes.JSON(adaptedContent),
	}).Error)

	result, err := s.PublishProject(project.ID, "wechat", &user.ID, uuid.Nil)

	require.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])
	assert.Contains(t, string(fakePublisher.Config), mediaRef)
	assert.NotContains(t, string(fakePublisher.Config), "fake://get/")
	assert.Contains(t, string(fakePublisher.AdaptedContent), mediaRef)
	assert.NotContains(t, string(fakePublisher.AdaptedContent), "fake://get/")
}

func TestPublishProjectPassesDecryptedBrowserCookiesToPublisher(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &testsupport.FakePlatformPublisher{}
	publisher.Factory.Register("douyin", fakePublisher)
	defer publisher.Factory.Register("douyin", &publisher.DouyinPublisher{})
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	createConnectedPublishAccount(t, db, user.ID, "wechat", datatypes.JSON(`{"app_id":"wx","app_secret":"secret"}`))
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "douyin",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{}`),
		AdaptedContent: datatypes.JSON(`{"summary":"ready"}`),
	}
	require.NoError(t, db.Create(&pub).Error)

	cookies := []publisher.Cookie{
		{Name: "sessionid", Value: "secret-value", Domain: ".douyin.com", Path: "/", Secure: true},
		{Name: "sid_guard", Value: "guard-value", Domain: ".douyin.com", Path: "/", Secure: true},
		{Name: "passport_csrf_token", Value: "csrf-value", Domain: ".douyin.com", Path: "/", Secure: true},
	}
	require.NoError(t, publisher.NewCookieStore(db).Save(context.Background(), user.ID, "douyin", cookies, publisher.RemoteAccountProfile{
		Username: "creator",
	}))

	result, err := s.PublishProject(project.ID, "douyin", &user.ID, uuid.Nil)

	require.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])
	assert.Contains(t, string(fakePublisher.AccountCookies), "secret-value")
	assert.NotContains(t, string(fakePublisher.AccountCookies), "ciphertext")

	var passedCookies []publisher.Cookie
	require.NoError(t, json.Unmarshal(fakePublisher.AccountCookies, &passedCookies))
	assert.Equal(t, cookies, passedCookies)
}

func TestPublishProjectIgnoresBrowserSessionIDForAsyncPublishing(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &testsupport.FakePlatformPublisher{}
	publisher.Factory.Register("wechat", fakePublisher)
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	createConnectedPublishAccount(t, db, user.ID, "wechat", datatypes.JSON(`{"app_id":"wx","app_secret":"secret"}`))
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"summary":"ready"}`),
	}
	require.NoError(t, db.Create(&pub).Error)
	sessionID := uuid.New()
	require.NoError(t, db.Create(&models.RemoteBrowserSession{
		ID:        sessionID,
		UserID:    user.ID,
		Platform:  "wechat",
		Status:    models.BrowserSessionStatusReady,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().Add(15 * time.Minute).UTC(),
	}).Error)

	result, err := s.PublishProject(project.ID, "wechat", &user.ID, sessionID)

	require.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])
	assert.Empty(t, fakePublisher.RemoteURL)
}

func TestPublishProjectRequiresSavedCookiesForBrowserCookiePlatforms(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &testsupport.FakePlatformPublisher{}
	publisher.Factory.Register("douyin", fakePublisher)
	defer publisher.Factory.Register("douyin", &publisher.DouyinPublisher{})

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
		Platform:       "douyin",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{}`),
		AdaptedContent: datatypes.JSON(`{"summary":"ready"}`),
	}
	require.NoError(t, db.Create(&pub).Error)

	_, err := s.PublishProject(project.ID, "douyin", &user.ID, uuid.Nil)

	require.Error(t, err)
	require.ErrorIs(t, err, services.ErrInvalidPlatformAccount)
	assert.Empty(t, fakePublisher.AccountCookies)
}

func TestPublishProjectBlocksUnhealthyAccountAndCreatesNotification(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &testsupport.FakePlatformPublisher{}
	publisher.Factory.Register("wechat", fakePublisher)
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	workspaceID := models.PersonalWorkspaceID(user.ID)
	project := models.Project{
		UserID:        user.ID,
		WorkspaceID:   &workspaceID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	account := createConnectedPublishAccount(t, db, user.ID, "wechat", datatypes.JSON(`{"app_id":"wx","app_secret":"secret"}`))
	require.NoError(t, db.Model(&account).Updates(map[string]any{
		"status":        models.PlatformAccountStatusNeedsReauth,
		"health_status": models.PlatformAccountHealthNeedsReauth,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:         project.ID,
		Platform:          "wechat",
		PlatformAccountID: &account.ID,
		Enabled:           true,
		Status:            models.PublicationStatusAdapted,
		Config:            datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent:    datatypes.JSON(`{"format":"html","html":"ready"}`),
	}).Error)

	result, err := s.PublishProject(project.ID, "wechat", &user.ID, uuid.Nil)

	require.ErrorIs(t, err, services.ErrInvalidPlatformAccount)
	require.Nil(t, result)
	assert.Empty(t, fakePublisher.Config)

	var notification models.Notification
	require.NoError(t, db.First(&notification, "recipient_user_id = ? AND event_type = ?", user.ID, models.NotificationAccountNeedsReauth).Error)
	assert.Equal(t, workspaceID, notification.WorkspaceID)
	assert.Equal(t, models.NotificationStatusUnread, notification.Status)
	assert.Equal(t, "platform_account", notification.ResourceType)
	require.NotNil(t, notification.ResourceID)
	assert.Equal(t, account.ID, *notification.ResourceID)
}

func TestPublishProjectRequiresPrepublishSyncForPendingPublication(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &testsupport.FakePlatformPublisher{}
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

	result, err := s.PublishProject(project.ID, "wechat", &user.ID, uuid.Nil)

	require.ErrorIs(t, err, services.ErrPublicationRequiresSync)
	require.Nil(t, result)

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "id = ?", pub.ID).Error)
	assert.Equal(t, models.PublicationStatusPending, saved.Status)
	assert.Empty(t, saved.ErrorMessage)
}

func TestPublishProjectRequiresPrepublishSyncForSyncingPublication(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &testsupport.FakePlatformPublisher{}
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
		Status:         models.PublicationStatusSyncing,
		Config:         datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"format":"html","html":"ready"}`),
	}
	require.NoError(t, db.Create(&pub).Error)

	result, err := s.PublishProject(project.ID, "wechat", &user.ID, uuid.Nil)

	require.ErrorIs(t, err, services.ErrPublicationRequiresSync)
	require.Nil(t, result)
	assert.Empty(t, fakePublisher.Config)

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "id = ?", pub.ID).Error)
	assert.Equal(t, models.PublicationStatusSyncing, saved.Status)
	assert.Nil(t, saved.LastAttemptAt)
}

func TestPublishProjectAllowsProjectEditor(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &testsupport.FakePlatformPublisher{}
	publisher.Factory.Register("wechat", fakePublisher)
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	editor := models.User{Username: "editor", Email: "editor@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&editor).Error)
	project := models.Project{
		UserID:        owner.ID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	account := createConnectedPublishAccount(t, db, owner.ID, "wechat", datatypes.JSON(`{"app_id":"wx","app_secret":"secret"}`))
	require.NoError(t, db.Create(&models.PlatformAccountGrant{
		PlatformAccountID: account.ID,
		WorkspaceID:       *account.WorkspaceID,
		GranteeUserID:     &editor.ID,
		Role:              models.PlatformAccountGrantRolePublisher,
		CreatedBy:         owner.ID,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    editor.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: owner.ID,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"format":"html","html":"ready"}`),
	}).Error)

	result, err := s.PublishProject(project.ID, "wechat", &editor.ID, uuid.Nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	var config map[string]string
	require.NoError(t, json.Unmarshal(fakePublisher.Config, &config))
	assert.Equal(t, "Title", config["title"])
	assert.Equal(t, "wx", config["app_id"])
	assert.Equal(t, "secret", config["app_secret"])
}

func TestPublishProjectRejectsProjectViewer(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &testsupport.FakePlatformPublisher{}
	publisher.Factory.Register("wechat", fakePublisher)
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	viewer := models.User{Username: "viewer", Email: "viewer@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&viewer).Error)
	project := models.Project{
		UserID:        owner.ID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	createConnectedPublishAccount(t, db, owner.ID, "wechat", datatypes.JSON(`{"app_id":"wx","app_secret":"secret"}`))
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    viewer.ID,
		Role:      models.ProjectRoleViewer,
		CreatedBy: owner.ID,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"format":"html","html":"ready"}`),
	}).Error)

	result, err := s.PublishProject(project.ID, "wechat", &viewer.ID, uuid.Nil)

	require.ErrorIs(t, err, services.ErrForbidden)
	require.Nil(t, result)
	require.Empty(t, fakePublisher.Config)
}

func TestPublishProjectUsesSavedXOAuth2Credentials(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &testsupport.FakePlatformPublisher{}
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

	result, err := s.PublishProject(project.ID, "x", &user.ID, uuid.Nil)
	require.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])

	var config map[string]any
	require.NoError(t, json.Unmarshal(fakePublisher.Config, &config))
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

	db := testsupport.SetupTestDB()
	refreshedExpiresAt := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	provider := &testsupport.FakeXOAuth2Provider{
		Token: pkgx.OAuth2Token{
			AccessToken:  "new-oauth2-access",
			RefreshToken: "new-oauth2-refresh",
			Scope:        "tweet.read tweet.write users.read offline.access",
			ExpiresAt:    refreshedExpiresAt,
		},
	}
	s := services.NewDashboardServiceWithXOAuth2Provider(db, provider)
	fakePublisher := &testsupport.FakePlatformPublisher{}
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
	credentials, err := json.Marshal(map[string]any{
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

	result, err := s.PublishProject(project.ID, "x", &user.ID, uuid.Nil)
	require.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])
	assert.Equal(t, "oauth2-refresh", provider.RefreshToken)
	assert.Equal(t, "client-id", provider.RefreshConfig.ClientID)
	assert.Equal(t, "client-secret", provider.RefreshConfig.ClientSecret)
	assert.Empty(t, provider.RefreshConfig.RedirectURI)

	var config map[string]any
	require.NoError(t, json.Unmarshal(fakePublisher.Config, &config))
	assert.Equal(t, "oauth2", config["auth_type"])
	assert.Equal(t, "new-oauth2-access", config["oauth2_access_token"])
	assert.Equal(t, "new-oauth2-refresh", config["oauth2_refresh_token"])
	assert.Equal(t, "creator", config["username"])

	var account models.PlatformAccount
	require.NoError(t, db.First(&account, "user_id = ? AND platform = ?", user.ID, "x").Error)
	var savedCredentials map[string]any
	require.NoError(t, json.Unmarshal(account.Credentials, &savedCredentials))
	assert.Equal(t, "new-oauth2-access", savedCredentials["oauth2_access_token"])
	assert.Equal(t, "new-oauth2-refresh", savedCredentials["oauth2_refresh_token"])
	assert.Equal(t, "tweet.read tweet.write users.read offline.access", savedCredentials["oauth2_scope"])
}

func TestCreateXPostIntentReturnsManualPublishURL(t *testing.T) {
	db := testsupport.SetupTestDB()
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

func TestCreateXPostIntentRequiresPrepublishSyncForPendingPublication(t *testing.T) {
	db := testsupport.SetupTestDB()
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

	require.ErrorIs(t, err, services.ErrPublicationRequiresSync)
	require.Nil(t, result)

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "id = ?", pub.ID).Error)
	assert.Equal(t, models.PublicationStatusPending, saved.Status)
	assert.JSONEq(t, `{}`, string(saved.AdaptedContent))
}

func TestCreateXPostIntentAllowsProjectEditor(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	editor := models.User{Username: "editor", Email: "editor@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&editor).Error)
	project := models.Project{
		UserID:        owner.ID,
		Title:         "manual x",
		SourceContent: "<p>source content</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    editor.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: owner.ID,
	}).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "x",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"text":"hello x"}`),
	}
	require.NoError(t, db.Create(&pub).Error)

	result, err := s.CreateXPostIntent(project.ID, &editor.ID)

	require.NoError(t, err)
	require.NotNil(t, result)

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "id = ?", pub.ID).Error)
	assert.NotEmpty(t, saved.PublishURL)
}

func TestCreateXPostIntentRejectsProjectViewer(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	viewer := models.User{Username: "viewer", Email: "viewer@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&viewer).Error)
	project := models.Project{
		UserID:        owner.ID,
		Title:         "manual x",
		SourceContent: "<p>source content</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    viewer.ID,
		Role:      models.ProjectRoleViewer,
		CreatedBy: owner.ID,
	}).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "x",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"text":"hello x"}`),
	}
	require.NoError(t, db.Create(&pub).Error)

	result, err := s.CreateXPostIntent(project.ID, &viewer.ID)

	require.ErrorIs(t, err, services.ErrForbidden)
	require.Nil(t, result)

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "id = ?", pub.ID).Error)
	assert.Empty(t, saved.PublishURL)
}

func TestPublishProjectRejectsDisabledPublication(t *testing.T) {
	db := testsupport.SetupTestDB()
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

	_, err := s.PublishProject(project.ID, "wechat", &user.ID, uuid.Nil)
	require.ErrorIs(t, err, services.ErrPublicationDisabled)
}

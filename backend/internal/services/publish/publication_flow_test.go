package publish_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	pkgx "github.com/kurodakayn/mpp-backend/internal/pkg/x"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

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

	assert.NoError(t, err)
	assert.Equal(t, 2, len(results))

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
	assert.NoError(t, err)

	result, err := s.PublishProject(project.ID, "wechat", &user.ID, uuid.Nil)
	assert.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])

	var config map[string]string
	assert.NoError(t, json.Unmarshal(fakePublisher.Config, &config))
	assert.Equal(t, "wx-saved", config["app_id"])
	assert.Equal(t, "saved-secret", config["app_secret"])
	assert.Equal(t, "Title", config["title"])
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
	assert.ErrorIs(t, err, services.ErrInvalidPlatformAccount)
	assert.Empty(t, fakePublisher.AccountCookies)
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

	var config map[string]interface{}
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

	result, err := s.PublishProject(project.ID, "x", &user.ID, uuid.Nil)
	require.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])
	assert.Equal(t, "oauth2-refresh", provider.RefreshToken)
	assert.Equal(t, "client-id", provider.RefreshConfig.ClientID)
	assert.Equal(t, "client-secret", provider.RefreshConfig.ClientSecret)
	assert.Empty(t, provider.RefreshConfig.RedirectURI)

	var config map[string]interface{}
	require.NoError(t, json.Unmarshal(fakePublisher.Config, &config))
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
	assert.ErrorIs(t, err, services.ErrPublicationDisabled)
}

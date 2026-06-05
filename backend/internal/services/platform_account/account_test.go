//nolint:gosec // Test fixtures use fake credential strings.
package platformaccount_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	pkgx "github.com/kurodakayn/mpp-backend/internal/pkg/x"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestWechatAccountSettingsSaveMasksSecret(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	db.Create(&user)

	resp, err := s.UpsertWechatAccount(user.ID, dto.UpsertWechatAccountRequest{
		AppID:     "wx-app",
		AppSecret: "wx-secret",
	})
	require.NoError(t, err)
	assert.Equal(t, "wechat", resp.Platform)
	assert.Equal(t, "wx-app", resp.AppID)
	assert.True(t, resp.HasAppSecret)
	assert.Equal(t, models.PlatformAccountStatusUntested, resp.Status)

	saved, err := s.GetWechatAccount(user.ID)
	require.NoError(t, err)
	assert.Equal(t, "wx-app", saved.AppID)
	assert.True(t, saved.HasAppSecret)
}

func TestWechatAccountTestUsesSavedSecretAndUpdatesStatus(t *testing.T) {
	db := testsupport.SetupTestDB()
	tester := &testsupport.FakeWechatTester{
		Result: dto.WechatConnectionTestResponse{
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
	require.NoError(t, err)

	result, err := s.TestWechatAccount(user.ID, dto.TestWechatAccountRequest{
		AppID: "wx-app",
	})
	require.NoError(t, err)
	assert.True(t, result.Connected)
	assert.Equal(t, "wx-app", tester.AppID)
	assert.Equal(t, "wx-secret", tester.Secret)

	saved, err := s.GetWechatAccount(user.ID)
	require.NoError(t, err)
	assert.Equal(t, models.PlatformAccountStatusConnected, saved.Status)
	assert.Empty(t, saved.LastTestError)
}

func TestWechatAccountTestDoesNotPersistUnsavedCredentialsStatus(t *testing.T) {
	db := testsupport.SetupTestDB()
	testedAt := time.Now()
	tester := &testsupport.FakeWechatTester{
		Result: dto.WechatConnectionTestResponse{
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
	require.NoError(t, err)

	result, err := s.TestWechatAccount(user.ID, dto.TestWechatAccountRequest{
		AppID:     "wx-draft",
		AppSecret: "draft-secret",
	})
	require.NoError(t, err)
	assert.False(t, result.Connected)
	assert.Equal(t, "wx-draft", tester.AppID)
	assert.Equal(t, "draft-secret", tester.Secret)

	saved, err := s.GetWechatAccount(user.ID)
	require.NoError(t, err)
	assert.Equal(t, models.PlatformAccountStatusUntested, saved.Status)
	assert.Nil(t, saved.LastTestedAt)
	assert.Empty(t, saved.LastTestError)
}

func TestXAccountSettingsClearsUsernameAndMetadataWhenCredentialsChange(t *testing.T) {
	db := testsupport.SetupTestDB()
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
	require.NoError(t, err)

	var account models.PlatformAccount
	require.NoError(t, db.First(&account, "user_id = ? AND platform = ?", user.ID, "x").Error)
	require.NoError(t, db.Model(&account).Update("metadata", datatypes.JSON(`{"username":"oldmeta"}`)).Error)

	_, err = s.UpsertXAccount(user.ID, dto.UpsertXAccountRequest{
		APIKey:            "x-new-key",
		APISecret:         "x-new-secret",
		AccessToken:       "x-new-token",
		AccessTokenSecret: "x-new-token-secret",
	})
	require.NoError(t, err)

	saved, err := s.GetXAccount(user.ID)
	require.NoError(t, err)
	assert.Empty(t, saved.Username)
	assert.Equal(t, models.PlatformAccountStatusUntested, saved.Status)

	require.NoError(t, db.First(&account, "user_id = ? AND platform = ?", user.ID, "x").Error)
	var credentials map[string]string
	require.NoError(t, json.Unmarshal(account.Credentials, &credentials))
	assert.Equal(t, "x-new-key", credentials["api_key"])
	assert.Empty(t, credentials["username"])

	var metadata map[string]string
	require.NoError(t, json.Unmarshal(account.Metadata, &metadata))
	assert.Empty(t, metadata["username"])
}

func TestXOAuth2FlowStoresConnectedAccount(t *testing.T) {
	t.Setenv("X_OAUTH2_CLIENT_ID", "client-id")
	t.Setenv("X_OAUTH2_CLIENT_SECRET", "client-secret")

	db := testsupport.SetupTestDB()
	expiresAt := time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)
	provider := &testsupport.FakeXOAuth2Provider{
		Token: pkgx.OAuth2Token{
			AccessToken:  "oauth2-access",
			RefreshToken: "oauth2-refresh",
			Scope:        "tweet.read tweet.write users.read offline.access",
			ExpiresAt:    expiresAt,
		},
		User: pkgx.User{
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
	require.NotEmpty(t, provider.AuthState)
	require.NotEmpty(t, provider.AuthChallenge)
	assert.Equal(t, "client-id", provider.AuthConfig.ClientID)
	assert.Equal(t, "client-secret", provider.AuthConfig.ClientSecret)

	parsedAuthURL, err := url.Parse(authURL)
	require.NoError(t, err)
	state := parsedAuthURL.Query().Get("state")
	require.NotEmpty(t, state)

	resp, err := s.CompleteXOAuth2(context.Background(), state, "auth-code")
	require.NoError(t, err)

	assert.Equal(t, "auth-code", provider.ExchangeCode)
	assert.NotEmpty(t, provider.ExchangeVerifier)
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

func TestGetDouyinAccount(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)

	empty, err := s.GetDouyinAccount(user.ID)
	require.NoError(t, err)
	assert.Equal(t, "douyin", empty.Platform)
	assert.Equal(t, "unconfigured", empty.Status)

	require.NoError(t, db.Create(&models.PlatformAccount{
		UserID:    user.ID,
		Platform:  "douyin",
		Username:  "creator",
		AvatarURL: "https://example.com/avatar.png",
		Status:    models.PlatformAccountStatusConnected,
	}).Error)

	account, err := s.GetDouyinAccount(user.ID)
	require.NoError(t, err)
	assert.Equal(t, "creator", account.Username)
	assert.Equal(t, "https://example.com/avatar.png", account.AvatarURL)
	assert.Equal(t, models.PlatformAccountStatusConnected, account.Status)
}

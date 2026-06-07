//nolint:gosec // Test fixtures use fake credential strings.
package platformaccount_test

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

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	pkgx "github.com/kurodakayn/mpp-backend/internal/pkg/x"
	"github.com/kurodakayn/mpp-backend/internal/services"
	platformaccount "github.com/kurodakayn/mpp-backend/internal/services/platform_account"
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

func TestGetWechatAccountUsesWriterForScopedAccount(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := services.NewDashboardServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	userID := uuid.New()
	workspaceID := models.PersonalWorkspaceID(userID)
	user := models.User{ID: userID, Username: "account-routing-owner", Email: "account-routing-owner@example.com"}
	require.NoError(t, writer.Create(&user).Error)
	require.NoError(t, reader.Create(&user).Error)

	ownerID := userID
	writerAccount := models.PlatformAccount{
		ID:             uuid.New(),
		UserID:         userID,
		WorkspaceID:    &workspaceID,
		OwnerUserID:    &ownerID,
		Platform:       "wechat",
		Username:       "writer wechat",
		DisplayName:    "writer wechat",
		PlatformUserID: "wx-writer",
		ShareScope:     models.PlatformAccountSharePrivate,
		Status:         models.PlatformAccountStatusConnected,
		HealthStatus:   models.PlatformAccountHealthHealthy,
		Credentials:    datatypes.JSON([]byte(`{"app_id":"wx-writer","app_secret":"writer-secret"}`)),
	}
	staleReaderAccount := models.PlatformAccount{
		ID:             uuid.New(),
		UserID:         userID,
		WorkspaceID:    &workspaceID,
		OwnerUserID:    &ownerID,
		Platform:       "wechat",
		Username:       "reader wechat",
		DisplayName:    "reader wechat",
		PlatformUserID: "wx-reader",
		ShareScope:     models.PlatformAccountSharePrivate,
		Status:         models.PlatformAccountStatusConnected,
		HealthStatus:   models.PlatformAccountHealthHealthy,
		Credentials:    datatypes.JSON([]byte(`{"app_id":"wx-reader","app_secret":"reader-secret"}`)),
	}
	require.NoError(t, writer.Create(&writerAccount).Error)
	require.NoError(t, reader.Create(&staleReaderAccount).Error)

	account, err := s.GetWechatAccount(userID)
	require.NoError(t, err)
	assert.Equal(t, "wx-writer", account.AppID)
	assert.True(t, account.HasAppSecret)
}

func TestResolvePublicationAccountUsesWriterForScopedAccount(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := platformaccount.NewServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	userID := uuid.New()
	workspaceID := models.PersonalWorkspaceID(userID)
	user := models.User{ID: userID, Username: "publication-account-owner", Email: "publication-account-owner@example.com"}
	require.NoError(t, writer.Create(&user).Error)
	require.NoError(t, reader.Create(&user).Error)

	project := models.Project{
		ID:            uuid.New(),
		UserID:        userID,
		WorkspaceID:   &workspaceID,
		Title:         "Publication account routing",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, writer.Create(&project).Error)
	require.NoError(t, reader.Create(&project).Error)

	ownerID := userID
	writerAccount := models.PlatformAccount{
		ID:           uuid.New(),
		UserID:       userID,
		WorkspaceID:  &workspaceID,
		OwnerUserID:  &ownerID,
		Platform:     "douyin",
		Username:     "writer account",
		DisplayName:  "writer account",
		ShareScope:   models.PlatformAccountSharePrivate,
		Status:       models.PlatformAccountStatusConnected,
		HealthStatus: models.PlatformAccountHealthHealthy,
	}
	staleReaderAccount := models.PlatformAccount{
		ID:           uuid.New(),
		UserID:       userID,
		WorkspaceID:  &workspaceID,
		OwnerUserID:  &ownerID,
		Platform:     "douyin",
		Username:     "stale reader account",
		DisplayName:  "stale reader account",
		ShareScope:   models.PlatformAccountSharePrivate,
		Status:       models.PlatformAccountStatusConnected,
		HealthStatus: models.PlatformAccountHealthHealthy,
	}
	require.NoError(t, writer.Create(&writerAccount).Error)
	require.NoError(t, reader.Create(&staleReaderAccount).Error)

	pub := models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "douyin",
	}
	account, err := s.ResolvePublicationAccount(userID, &pub)
	require.NoError(t, err)
	require.NotNil(t, pub.PlatformAccountID)
	assert.Equal(t, writerAccount.ID, account.ID)
	assert.Equal(t, writerAccount.ID, *pub.PlatformAccountID)
}

func TestRequireAccountUseUsesWriterForWorkspaceMembership(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := platformaccount.NewServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	actorID := uuid.New()
	ownerID := uuid.New()
	workspaceID := uuid.New()
	account := models.PlatformAccount{
		ID:           uuid.New(),
		UserID:       ownerID,
		WorkspaceID:  &workspaceID,
		OwnerUserID:  &ownerID,
		Platform:     "douyin",
		Username:     "workspace shared",
		DisplayName:  "workspace shared",
		ShareScope:   models.PlatformAccountShareWorkspace,
		Status:       models.PlatformAccountStatusConnected,
		HealthStatus: models.PlatformAccountHealthHealthy,
	}
	require.NoError(t, reader.Create(&models.WorkspaceMember{
		WorkspaceID: workspaceID,
		UserID:      actorID,
		Role:        models.WorkspaceRoleMember,
	}).Error)

	err := s.RequireAccountUse(actorID, account, uuid.Nil)
	require.ErrorIs(t, err, platformaccount.ErrPlatformAccountForbidden)
}

func TestRequireAccountUseUsesWriterForAccountGrant(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := platformaccount.NewServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))

	actorID := uuid.New()
	accountID := uuid.New()
	account := models.PlatformAccount{
		ID:           accountID,
		UserID:       uuid.New(),
		Platform:     "douyin",
		Username:     "private account",
		DisplayName:  "private account",
		ShareScope:   models.PlatformAccountSharePrivate,
		Status:       models.PlatformAccountStatusConnected,
		HealthStatus: models.PlatformAccountHealthHealthy,
	}
	require.NoError(t, reader.Create(&models.PlatformAccountGrant{
		ID:                uuid.New(),
		PlatformAccountID: accountID,
		WorkspaceID:       uuid.New(),
		GranteeUserID:     &actorID,
		Role:              models.PlatformAccountGrantRolePublisher,
		CreatedBy:         uuid.New(),
	}).Error)

	err := s.RequireAccountUse(actorID, account, uuid.Nil)
	require.ErrorIs(t, err, platformaccount.ErrPlatformAccountForbidden)
}

func TestResolvePublicationAccountFallbackSkipsInaccessibleAccount(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := platformaccount.NewService(db)
	now := time.Now()

	owner := models.User{Username: "owner"}
	actor := models.User{Username: "actor"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&actor).Error)

	workspace := models.Workspace{
		OwnerUserID: owner.ID,
		Name:        "Team",
		Slug:        "team",
		Status:      models.WorkspaceStatusActive,
	}
	require.NoError(t, db.Create(&workspace).Error)
	require.NoError(t, db.Create(&models.WorkspaceMember{
		WorkspaceID: workspace.ID,
		UserID:      actor.ID,
		Role:        models.WorkspaceRoleMember,
	}).Error)

	project := models.Project{
		UserID:        owner.ID,
		WorkspaceID:   &workspace.ID,
		Title:         "Project",
		SourceContent: "content",
		Status:        models.ProjectStatusDraft,
	}
	require.NoError(t, db.Create(&project).Error)

	privateOwnerID := owner.ID
	newerPrivate := models.PlatformAccount{
		UserID:         owner.ID,
		WorkspaceID:    &workspace.ID,
		OwnerUserID:    &privateOwnerID,
		Platform:       "douyin",
		Username:       "private",
		DisplayName:    "private",
		ShareScope:     models.PlatformAccountSharePrivate,
		Status:         models.PlatformAccountStatusConnected,
		HealthStatus:   models.PlatformAccountHealthHealthy,
		LastTestedAt:   &now,
		LastVerifiedAt: &now,
	}
	require.NoError(t, db.Create(&newerPrivate).Error)
	require.NoError(t, db.Model(&newerPrivate).Update("updated_at", now.Add(time.Minute)).Error)

	actorOwnerID := actor.ID
	olderOwned := models.PlatformAccount{
		UserID:         actor.ID,
		WorkspaceID:    &workspace.ID,
		OwnerUserID:    &actorOwnerID,
		Platform:       "douyin",
		Username:       "usable",
		DisplayName:    "usable",
		ShareScope:     models.PlatformAccountSharePrivate,
		Status:         models.PlatformAccountStatusConnected,
		HealthStatus:   models.PlatformAccountHealthHealthy,
		LastTestedAt:   &now,
		LastVerifiedAt: &now,
	}
	require.NoError(t, db.Create(&olderOwned).Error)
	require.NoError(t, db.Model(&olderOwned).Update("updated_at", now).Error)

	pub := models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "douyin",
	}
	account, err := s.ResolvePublicationAccount(actor.ID, &pub)
	require.NoError(t, err)
	require.NotNil(t, pub.PlatformAccountID)
	assert.Equal(t, olderOwned.ID, account.ID)
	assert.Equal(t, olderOwned.ID, *pub.PlatformAccountID)
}

func TestRequireAccountUseRequiresCurrentWorkspaceMembershipForOwner(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := platformaccount.NewService(db)

	user := models.User{Username: "former-member"}
	require.NoError(t, db.Create(&user).Error)
	workspace := models.Workspace{
		OwnerUserID: user.ID,
		Name:        "Former Team",
		Slug:        "former-team",
		Status:      models.WorkspaceStatusActive,
	}
	require.NoError(t, db.Create(&workspace).Error)

	ownerID := user.ID
	account := models.PlatformAccount{
		UserID:       user.ID,
		WorkspaceID:  &workspace.ID,
		OwnerUserID:  &ownerID,
		Platform:     "douyin",
		Username:     "owner",
		DisplayName:  "owner",
		ShareScope:   models.PlatformAccountSharePrivate,
		Status:       models.PlatformAccountStatusConnected,
		HealthStatus: models.PlatformAccountHealthHealthy,
	}
	require.NoError(t, db.Create(&account).Error)

	err := s.RequireAccountUse(user.ID, account, uuid.Nil)
	require.ErrorIs(t, err, platformaccount.ErrPlatformAccountForbidden)
}

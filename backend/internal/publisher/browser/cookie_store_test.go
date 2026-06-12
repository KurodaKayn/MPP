package browser

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

func TestCookieStore(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.PlatformAccount{})
	require.NoError(t, err)

	store := NewCookieStore(db)
	userID := uuid.New()
	platform := "douyin"
	encryptionKey := "12345678901234567890123456789012" // 32 bytes

	t.Run("Missing Encryption Key", func(t *testing.T) {
		t.Setenv("COOKIE_ENCRYPTION_KEY", "")
		err := store.Save(context.Background(), userID, platform, []Cookie{}, RemoteAccountProfile{})
		require.ErrorIs(t, err, ErrCookieEncryptionKeyMissing)
	})

	t.Run("Invalid Encryption Key Length", func(t *testing.T) {
		t.Setenv("COOKIE_ENCRYPTION_KEY", "too-short")
		err := store.Save(context.Background(), userID, platform, []Cookie{}, RemoteAccountProfile{})
		require.ErrorIs(t, err, ErrCookieEncryptionKeyInvalid)
	})

	t.Run("Full Cycle", func(t *testing.T) {
		t.Setenv("COOKIE_ENCRYPTION_KEY", encryptionKey)
		cookies := []Cookie{
			{Name: "sessionid", Value: "secret-value", Domain: ".douyin.com", Path: "/", Secure: true, HttpOnly: true, SameSite: "None"},
			{Name: "sid_guard", Value: "guard-value", Domain: "creator.douyin.com", Path: "/", Secure: true, HttpOnly: true},
			{Name: "passport_csrf_token", Value: "csrf-value", Domain: ".douyin.com", Path: "/", Secure: true},
			{Name: "ignored", Value: "tracking-value", Domain: ".douyin.com", Path: "/"},
			{Name: "sessionid", Value: "evil-value", Domain: "douyin.com.evil.test", Path: "/"},
		}
		expectedCookies := []Cookie{
			{Name: "sessionid", Value: "secret-value", Domain: ".douyin.com", Path: "/", Secure: true, HttpOnly: true, SameSite: "None"},
			{Name: "sid_guard", Value: "guard-value", Domain: "creator.douyin.com", Path: "/", Secure: true, HttpOnly: true},
			{Name: "passport_csrf_token", Value: "csrf-value", Domain: ".douyin.com", Path: "/", Secure: true},
		}
		profile := RemoteAccountProfile{
			Username:  "testuser",
			AvatarURL: "https://example.com/avatar.png",
		}

		// Test Save
		err = store.Save(context.Background(), userID, platform, cookies, profile)
		require.NoError(t, err)

		// Verify encryption in DB
		var account models.PlatformAccount
		err = db.Where("user_id = ? AND platform = ?", userID, platform).First(&account).Error
		require.NoError(t, err)
		assert.NotContains(t, string(account.Cookies), "secret-value") // Should be encrypted
		assert.Contains(t, string(account.Cookies), "ciphertext")
		assert.Equal(t, "testuser", account.Username)
		assert.Equal(t, "https://example.com/avatar.png", account.AvatarURL)

		// Test Load
		loadedCookies, err := store.Load(context.Background(), userID, platform)
		require.NoError(t, err)
		assert.Equal(t, expectedCookies, loadedCookies)

		testedAt := time.Now()
		require.NoError(t, db.Model(&account).Updates(map[string]any{
			"last_tested_at":  testedAt,
			"last_test_error": "previous validation failed",
			"metadata":        datatypes.JSON([]byte(`{"profile":"cached"}`)),
		}).Error)

		// Test Delete
		err = store.Delete(context.Background(), userID, platform)
		require.NoError(t, err)

		_, err = store.Load(context.Background(), userID, platform)
		require.ErrorIs(t, err, ErrCookieNotFound)

		var deletedAccount models.PlatformAccount
		err = db.Where("user_id = ? AND platform = ?", userID, platform).First(&deletedAccount).Error
		require.NoError(t, err)
		assert.Equal(t, models.PlatformAccountStatusUntested, deletedAccount.Status)
		assert.Empty(t, deletedAccount.Username)
		assert.Empty(t, deletedAccount.AvatarURL)
		assert.Nil(t, deletedAccount.LastTestedAt)
		assert.Empty(t, deletedAccount.LastTestError)
		assert.JSONEq(t, `{}`, string(deletedAccount.Metadata))
	})

	t.Run("Decryption Failure with Wrong Key", func(t *testing.T) {
		t.Setenv("COOKIE_ENCRYPTION_KEY", encryptionKey)
		wrongKeyUserID := uuid.New()
		cookies := []Cookie{
			{Name: "sessionid", Value: "secret-value", Domain: ".douyin.com", Path: "/"},
			{Name: "sid_guard", Value: "guard-value", Domain: ".douyin.com", Path: "/"},
			{Name: "passport_csrf_token", Value: "csrf-value", Domain: ".douyin.com", Path: "/"},
		}
		err := store.Save(context.Background(), wrongKeyUserID, "douyin", cookies, RemoteAccountProfile{})
		require.NoError(t, err)

		// Change key
		t.Setenv("COOKIE_ENCRYPTION_KEY", "another-32-byte-key-012345678901")
		_, err = store.Load(context.Background(), wrongKeyUserID, "douyin")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt cookies")
	})

	t.Run("Rejects Missing Required Cookies", func(t *testing.T) {
		t.Setenv("COOKIE_ENCRYPTION_KEY", encryptionKey)
		err := store.Save(context.Background(), userID, "douyin", []Cookie{
			{Name: "sessionid", Value: "secret-value", Domain: ".douyin.com", Path: "/"},
		}, RemoteAccountProfile{})
		require.ErrorIs(t, err, ErrCookieValidationFailed)
	})

	t.Run("Workspace Save Without Account ID Creates Distinct Remote Account", func(t *testing.T) {
		t.Setenv("COOKIE_ENCRYPTION_KEY", encryptionKey)
		workspaceID := uuid.New()
		accountUserID := uuid.New()
		existing := models.PlatformAccount{
			UserID:         accountUserID,
			WorkspaceID:    &workspaceID,
			Platform:       platform,
			Username:       "existing",
			DisplayName:    "existing",
			PlatformUserID: "remote-existing",
			Cookies:        datatypes.JSON([]byte(`[]`)),
			Status:         models.PlatformAccountStatusConnected,
			HealthStatus:   models.PlatformAccountHealthHealthy,
		}
		require.NoError(t, db.Create(&existing).Error)

		cookies := []Cookie{
			{Name: "sessionid", Value: "new-secret", Domain: ".douyin.com", Path: "/"},
			{Name: "sid_guard", Value: "new-guard", Domain: ".douyin.com", Path: "/"},
			{Name: "passport_csrf_token", Value: "new-csrf", Domain: ".douyin.com", Path: "/"},
		}
		err := store.SaveForAccount(context.Background(), accountUserID, workspaceID, uuid.Nil, platform, cookies, RemoteAccountProfile{
			Username:       "new-account",
			PlatformUserID: "remote-new",
		})
		require.NoError(t, err)

		var accounts []models.PlatformAccount
		require.NoError(t, db.Where("workspace_id = ? AND platform = ?", workspaceID, platform).Order("username").Find(&accounts).Error)
		require.Len(t, accounts, 2)

		var unchanged models.PlatformAccount
		require.NoError(t, db.First(&unchanged, "id = ?", existing.ID).Error)
		assert.Equal(t, "existing", unchanged.Username)
		assert.JSONEq(t, `[]`, string(unchanged.Cookies))

		var created models.PlatformAccount
		require.NoError(t, db.First(&created, "workspace_id = ? AND platform = ? AND platform_user_id = ?", workspaceID, platform, "remote-new").Error)
		assert.Equal(t, "new-account", created.Username)
		assert.Contains(t, string(created.Cookies), "ciphertext")
	})

	t.Run("Account ID Save Requires Matching Workspace", func(t *testing.T) {
		t.Setenv("COOKIE_ENCRYPTION_KEY", encryptionKey)
		victimWorkspaceID := uuid.New()
		attackerWorkspaceID := uuid.New()
		accountUserID := uuid.New()
		existing := models.PlatformAccount{
			UserID:       accountUserID,
			WorkspaceID:  &victimWorkspaceID,
			Platform:     platform,
			Username:     "existing",
			DisplayName:  "existing",
			Cookies:      datatypes.JSON([]byte(`[]`)),
			Status:       models.PlatformAccountStatusConnected,
			HealthStatus: models.PlatformAccountHealthHealthy,
		}
		require.NoError(t, db.Create(&existing).Error)

		cookies := []Cookie{
			{Name: "sessionid", Value: "new-secret", Domain: ".douyin.com", Path: "/"},
			{Name: "sid_guard", Value: "new-guard", Domain: ".douyin.com", Path: "/"},
			{Name: "passport_csrf_token", Value: "new-csrf", Domain: ".douyin.com", Path: "/"},
		}
		err := store.SaveForAccount(context.Background(), accountUserID, attackerWorkspaceID, existing.ID, platform, cookies, RemoteAccountProfile{
			Username: "attacker-update",
		})
		require.ErrorIs(t, err, ErrCookieNotFound)

		var unchanged models.PlatformAccount
		require.NoError(t, db.First(&unchanged, "id = ?", existing.ID).Error)
		assert.Equal(t, "existing", unchanged.Username)
		assert.JSONEq(t, `[]`, string(unchanged.Cookies))
	})

	t.Run("Workspace Save Without Remote ID Updates Matching Display Account", func(t *testing.T) {
		t.Setenv("COOKIE_ENCRYPTION_KEY", encryptionKey)
		workspaceID := uuid.New()
		accountUserID := uuid.New()
		existing := models.PlatformAccount{
			UserID:       accountUserID,
			WorkspaceID:  &workspaceID,
			Platform:     "zhihu",
			Username:     "点击打开zhiybDlu的主页",
			DisplayName:  "点击打开zhiybDlu的主页",
			Cookies:      datatypes.JSON([]byte(`[]`)),
			Status:       models.PlatformAccountStatusUntested,
			HealthStatus: models.PlatformAccountHealthUnknown,
		}
		require.NoError(t, db.Create(&existing).Error)

		cookies := []Cookie{
			{Name: "z_c0", Value: "secret-value", Domain: ".zhihu.com", Path: "/"},
		}
		err := store.SaveForAccount(context.Background(), accountUserID, workspaceID, uuid.Nil, "zhihu", cookies, RemoteAccountProfile{
			Username: "点击打开zhiybDlu的主页",
		})
		require.NoError(t, err)

		var accounts []models.PlatformAccount
		require.NoError(t, db.Where("workspace_id = ? AND platform = ? AND display_name = ?", workspaceID, "zhihu", "点击打开zhiybDlu的主页").Find(&accounts).Error)
		require.Len(t, accounts, 1)
		assert.Equal(t, existing.ID, accounts[0].ID)
		assert.Equal(t, models.PlatformAccountStatusConnected, accounts[0].Status)
		assert.Equal(t, models.PlatformAccountHealthHealthy, accounts[0].HealthStatus)
		assert.Contains(t, string(accounts[0].Cookies), "ciphertext")
	})
}

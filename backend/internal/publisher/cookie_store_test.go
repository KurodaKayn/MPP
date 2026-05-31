package publisher

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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
		assert.ErrorIs(t, err, ErrCookieEncryptionKeyMissing)
	})

	t.Run("Invalid Encryption Key Length", func(t *testing.T) {
		t.Setenv("COOKIE_ENCRYPTION_KEY", "too-short")
		err := store.Save(context.Background(), userID, platform, []Cookie{}, RemoteAccountProfile{})
		assert.ErrorIs(t, err, ErrCookieEncryptionKeyInvalid)
	})

	t.Run("Full Cycle", func(t *testing.T) {
		t.Setenv("COOKIE_ENCRYPTION_KEY", encryptionKey)
		cookies := []Cookie{
			{Name: "sessionid", Value: "secret-value", Domain: ".douyin.com", Path: "/", Secure: true, HttpOnly: true},
			{Name: "sid_guard", Value: "guard-value", Domain: "creator.douyin.com", Path: "/", Secure: true, HttpOnly: true},
			{Name: "passport_csrf_token", Value: "csrf-value", Domain: ".douyin.com", Path: "/", Secure: true},
			{Name: "ignored", Value: "tracking-value", Domain: ".douyin.com", Path: "/"},
			{Name: "sessionid", Value: "evil-value", Domain: "douyin.com.evil.test", Path: "/"},
		}
		expectedCookies := []Cookie{
			{Name: "sessionid", Value: "secret-value", Domain: ".douyin.com", Path: "/", Secure: true, HttpOnly: true},
			{Name: "sid_guard", Value: "guard-value", Domain: "creator.douyin.com", Path: "/", Secure: true, HttpOnly: true},
			{Name: "passport_csrf_token", Value: "csrf-value", Domain: ".douyin.com", Path: "/", Secure: true},
		}
		profile := RemoteAccountProfile{
			Username:  "testuser",
			AvatarURL: "https://example.com/avatar.png",
		}

		// Test Save
		err = store.Save(context.Background(), userID, platform, cookies, profile)
		assert.NoError(t, err)

		// Verify encryption in DB
		var account models.PlatformAccount
		err = db.Where("user_id = ? AND platform = ?", userID, platform).First(&account).Error
		assert.NoError(t, err)
		assert.NotContains(t, string(account.Cookies), "secret-value") // Should be encrypted
		assert.Contains(t, string(account.Cookies), "ciphertext")
		assert.Equal(t, "testuser", account.Username)
		assert.Equal(t, "https://example.com/avatar.png", account.AvatarURL)

		// Test Load
		loadedCookies, err := store.Load(context.Background(), userID, platform)
		assert.NoError(t, err)
		assert.Equal(t, expectedCookies, loadedCookies)

		// Test Delete
		err = store.Delete(context.Background(), userID, platform)
		assert.NoError(t, err)

		_, err = store.Load(context.Background(), userID, platform)
		assert.ErrorIs(t, err, ErrCookieNotFound)
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
		assert.NoError(t, err)

		// Change key
		t.Setenv("COOKIE_ENCRYPTION_KEY", "another-32-byte-key-012345678901")
		_, err = store.Load(context.Background(), wrongKeyUserID, "douyin")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt cookies")
	})

	t.Run("Rejects Missing Required Cookies", func(t *testing.T) {
		t.Setenv("COOKIE_ENCRYPTION_KEY", encryptionKey)
		err := store.Save(context.Background(), userID, "douyin", []Cookie{
			{Name: "sessionid", Value: "secret-value", Domain: ".douyin.com", Path: "/"},
		}, RemoteAccountProfile{})
		assert.ErrorIs(t, err, ErrCookieValidationFailed)
	})
}

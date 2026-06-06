package browser

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

var (
	ErrCookieEncryptionKeyMissing = errors.New("COOKIE_ENCRYPTION_KEY is not configured")
	ErrCookieEncryptionKeyInvalid = errors.New("COOKIE_ENCRYPTION_KEY must be 32 bytes for AES-256")
	ErrCookieValidationFailed     = errors.New("required cookies are missing or expired")
	ErrCookieNotFound             = errors.New("no saved cookies exist for the user/platform")
)

type EncryptedEnvelope struct {
	Version    int    `json:"version"`
	Alg        string `json:"alg"`
	KID        string `json:"kid"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

type CookieStore struct {
	db *gorm.DB
}

func NewCookieStore(db *gorm.DB) *CookieStore {
	return &CookieStore{db: db}
}

func (s *CookieStore) WithContext(ctx context.Context) *CookieStore {
	if ctx == nil {
		return s
	}
	return &CookieStore{db: s.db.WithContext(ctx)}
}

func (s *CookieStore) Save(ctx context.Context, userID uuid.UUID, platform string, cookies []Cookie, profile RemoteAccountProfile) error {
	return s.SaveForAccount(ctx, userID, uuid.Nil, uuid.Nil, platform, cookies, profile)
}

func (s *CookieStore) SaveForAccount(ctx context.Context, userID uuid.UUID, workspaceID uuid.UUID, accountID uuid.UUID, platform string, cookies []Cookie, profile RemoteAccountProfile) error {
	key := os.Getenv("COOKIE_ENCRYPTION_KEY")
	if key == "" {
		return ErrCookieEncryptionKeyMissing
	}
	if len(key) != 32 {
		return ErrCookieEncryptionKeyInvalid
	}

	normalizedCookies, err := NormalizePlatformCookies(platform, cookies)
	if err != nil {
		return err
	}

	plaintext, err := json.Marshal(normalizedCookies)
	if err != nil {
		return fmt.Errorf("failed to marshal cookies: %w", err)
	}

	ciphertext, nonce, err := encrypt(plaintext, []byte(key))
	if err != nil {
		return fmt.Errorf("failed to encrypt cookies: %w", err)
	}

	envelope := EncryptedEnvelope{
		Version:    1,
		Alg:        "AES-256-GCM",
		KID:        "default",
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}

	envelopeJSON, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("failed to marshal envelope: %w", err)
	}

	now := time.Now().UTC()
	updates := map[string]any{
		"cookies":              datatypes.JSON(envelopeJSON),
		"username":             profile.Username,
		"display_name":         firstNonEmpty(profile.Username, platform),
		"avatar_url":           profile.AvatarURL,
		"status":               models.PlatformAccountStatusConnected,
		"health_status":        models.PlatformAccountHealthHealthy,
		"connected_by_user_id": userID,
		"last_connected_at":    &now,
		"last_verified_at":     &now,
		"last_tested_at":       &now,
		"last_test_error":      "",
	}
	if workspaceID != uuid.Nil {
		updates["workspace_id"] = workspaceID
	}

	query := s.db.WithContext(ctx).Model(&models.PlatformAccount{})
	if accountID != uuid.Nil {
		query = query.Where("id = ?", accountID)
	} else if workspaceID != uuid.Nil {
		query = query.Where("workspace_id = ? AND platform = ?", workspaceID, platform)
	} else {
		query = query.Where("user_id = ? AND platform = ?", userID, platform)
	}
	result := query.Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to update platform account: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// Create if not exists? Usually it should exist by the time we have a browser session.
		// For safety, let's assume it might not exist if we allow direct connection.
		account := models.PlatformAccount{
			UserID:              userID,
			Platform:            platform,
			Username:            profile.Username,
			DisplayName:         firstNonEmpty(profile.Username, platform),
			AvatarURL:           profile.AvatarURL,
			Cookies:             datatypes.JSON(envelopeJSON),
			Status:              models.PlatformAccountStatusConnected,
			HealthStatus:        models.PlatformAccountHealthHealthy,
			Credentials:         datatypes.JSON([]byte("{}")),
			Metadata:            datatypes.JSON([]byte("{}")),
			Config:              datatypes.JSON([]byte("{}")),
			ConnectedByUserID:   &userID,
			LastConnectedAt:     &now,
			LastVerifiedAt:      &now,
			LastTestedAt:        &now,
			CredentialSecretRef: "",
		}
		ownerID := userID
		account.OwnerUserID = &ownerID
		if workspaceID == uuid.Nil {
			workspaceID = models.PersonalWorkspaceID(userID)
		}
		account.WorkspaceID = &workspaceID
		if err := s.db.WithContext(ctx).Create(&account).Error; err != nil {
			return fmt.Errorf("failed to create platform account: %w", err)
		}
	}

	return nil
}

func (s *CookieStore) Load(ctx context.Context, userID uuid.UUID, platform string) ([]Cookie, error) {
	return s.LoadForAccount(ctx, userID, uuid.Nil, platform)
}

func (s *CookieStore) LoadForAccount(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, platform string) ([]Cookie, error) {
	var account models.PlatformAccount
	query := s.db.WithContext(ctx)
	if accountID != uuid.Nil {
		query = query.Where("id = ?", accountID)
	} else {
		query = query.Where("user_id = ? AND platform = ?", userID, platform)
	}
	err := query.First(&account).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCookieNotFound
		}
		return nil, err
	}

	if len(account.Cookies) == 0 || string(account.Cookies) == "[]" || string(account.Cookies) == "{}" {
		return nil, ErrCookieNotFound
	}

	var envelope EncryptedEnvelope
	if err := json.Unmarshal(account.Cookies, &envelope); err != nil {
		// Fallback for non-encrypted cookies if any (from old version)
		var cookies []Cookie
		if err := json.Unmarshal(account.Cookies, &cookies); err == nil {
			return NormalizePlatformCookies(platform, cookies)
		}
		return nil, fmt.Errorf("failed to unmarshal envelope: %w", err)
	}

	// If it's not actually an envelope (e.g. version 0 or wrong format), version check
	if envelope.Version != 1 || envelope.Alg != "AES-256-GCM" {
		return nil, fmt.Errorf("unsupported cookie envelope version or algorithm")
	}

	key := os.Getenv("COOKIE_ENCRYPTION_KEY")
	if key == "" {
		return nil, ErrCookieEncryptionKeyMissing
	}
	if len(key) != 32 {
		return nil, ErrCookieEncryptionKeyInvalid
	}

	plaintext, err := decrypt(envelope.Ciphertext, []byte(key), envelope.Nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt cookies: %w", err)
	}

	var cookies []Cookie
	if err := json.Unmarshal(plaintext, &cookies); err != nil {
		return nil, fmt.Errorf("failed to unmarshal decrypted cookies: %w", err)
	}

	return NormalizePlatformCookies(platform, cookies)
}

func (s *CookieStore) Delete(ctx context.Context, userID uuid.UUID, platform string) error {
	return s.db.WithContext(ctx).Model(&models.PlatformAccount{}).
		Where("user_id = ? AND platform = ?", userID, platform).
		Select("cookies", "status", "username", "avatar_url", "metadata", "last_tested_at", "last_test_error").
		Updates(map[string]any{
			"cookies":         datatypes.JSON([]byte("[]")),
			"status":          models.PlatformAccountStatusUntested,
			"username":        "",
			"avatar_url":      "",
			"metadata":        datatypes.JSON([]byte("{}")),
			"last_tested_at":  nil,
			"last_test_error": "",
		}).Error
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func NormalizePlatformCookies(platform string, cookies []Cookie) ([]Cookie, error) {
	requirements := cookieRequirementsForPlatform(platform)
	if len(requirements) == 0 {
		return nil, ErrCookieValidationFailed
	}

	now := time.Now()
	normalized := make([]Cookie, 0, len(cookies))
	seen := make(map[string]int)
	for _, cookie := range cookies {
		if cookie.Name == "" || cookie.Value == "" || cookieExpired(cookie, now) {
			continue
		}
		if !preserveCookie(cookie, requirements) {
			continue
		}
		if cookie.Path == "" {
			cookie.Path = "/"
		}
		key := strings.ToLower(cookie.Name + "\x00" + cookie.Domain + "\x00" + cookie.Path)
		if existing, ok := seen[key]; ok {
			normalized[existing] = cookie
			continue
		}
		seen[key] = len(normalized)
		normalized = append(normalized, cookie)
	}

	if missingRequiredCookies(normalized, requirements, now) {
		return nil, ErrCookieValidationFailed
	}
	return normalized, nil
}

func cookieRequirementsForPlatform(platform string) []CookieRequirement {
	switch platform {
	case "douyin":
		return []CookieRequirement{
			{Name: "sessionid", DomainSuffixes: []string{".douyin.com"}, Required: true, Preserve: true},
			{Name: "sid_guard", DomainSuffixes: []string{".douyin.com"}, Required: true, Preserve: true},
			{Name: "passport_csrf_token", DomainSuffixes: []string{".douyin.com"}, Required: true, Preserve: true},
		}
	case "zhihu":
		return []CookieRequirement{
			{Name: "z_c0", DomainSuffixes: []string{".zhihu.com"}, Required: true, Preserve: true},
			{Name: "q_c1", DomainSuffixes: []string{".zhihu.com"}, Required: false, Preserve: true},
			{Name: "d_c0", DomainSuffixes: []string{".zhihu.com"}, Required: false, Preserve: true},
		}
	default:
		return nil
	}
}

func preserveCookie(cookie Cookie, requirements []CookieRequirement) bool {
	for _, req := range requirements {
		if !req.Required && !req.Preserve {
			continue
		}
		if cookieMatchesRequirement(cookie, req) {
			return true
		}
	}
	return false
}

func missingRequiredCookies(cookies []Cookie, requirements []CookieRequirement, now time.Time) bool {
	for _, req := range requirements {
		if !req.Required {
			continue
		}
		found := false
		for _, cookie := range cookies {
			if cookieExpired(cookie, now) {
				continue
			}
			if cookieMatchesRequirement(cookie, req) {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
}

func cookieMatchesRequirement(cookie Cookie, req CookieRequirement) bool {
	if cookie.Name != req.Name || cookie.Value == "" {
		return false
	}
	for _, suffix := range req.DomainSuffixes {
		if cookieDomainMatches(cookie.Domain, suffix) {
			return true
		}
	}
	return false
}

func cookieExpired(cookie Cookie, now time.Time) bool {
	return cookie.Expires > 0 && !time.Unix(int64(cookie.Expires), 0).After(now)
}

func cookieDomainMatches(domain, suffix string) bool {
	domain = strings.TrimPrefix(strings.ToLower(domain), ".")
	suffix = strings.TrimPrefix(strings.ToLower(suffix), ".")
	return domain == suffix || strings.HasSuffix(domain, "."+suffix)
}

func encrypt(plaintext []byte, key []byte) (ciphertext []byte, nonce []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}

	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}

	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

func decrypt(ciphertext []byte, key []byte, nonce []byte) (plaintext []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(nonce) != gcm.NonceSize() {
		return nil, errors.New("invalid nonce size")
	}

	return gcm.Open(nil, nonce, ciphertext, nil)
}

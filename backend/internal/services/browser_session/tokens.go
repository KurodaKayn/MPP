package browsersession

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

type browserStreamTokenMeta struct {
	SessionID uuid.UUID `json:"session_id"`
	UserID    uuid.UUID `json:"user_id"`
	Platform  string    `json:"platform"`
	Purpose   string    `json:"purpose"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *BrowserSessionService) rotateRedisStreamToken(ctx context.Context, sessionID uuid.UUID, userID uuid.UUID, platform string, tokenHash string, sessionExpiresAt time.Time) (time.Time, error) {
	expiresAt := StreamTokenExpiresAt(sessionExpiresAt)
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return time.Time{}, ErrInvalidStreamToken
	}
	if s.coordinationRedisClient == nil {
		return expiresAt, nil
	}
	meta := browserStreamTokenMeta{
		SessionID: sessionID,
		UserID:    userID,
		Platform:  platform,
		Purpose:   "stream",
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: expiresAt,
	}
	payload, err := json.Marshal(meta)
	if err != nil {
		return time.Time{}, err
	}
	currentKey := browserSessionStreamCurrentKey(sessionID)
	oldHash, err := s.coordinationRedisClient.Get(ctx, currentKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return time.Time{}, err
	}
	tokenKey := browserSessionStreamTokenKey(sessionID, tokenHash)
	if err := s.coordinationRedisClient.Set(ctx, tokenKey, payload, ttl).Err(); err != nil {
		return time.Time{}, err
	}
	if err := s.coordinationRedisClient.Set(ctx, currentKey, tokenHash, ttl).Err(); err != nil {
		_ = s.coordinationRedisClient.Del(ctx, tokenKey).Err()
		return time.Time{}, err
	}
	if oldHash != "" && oldHash != tokenHash {
		_ = s.coordinationRedisClient.Del(ctx, browserSessionStreamTokenKey(sessionID, oldHash)).Err()
	}
	return expiresAt, nil
}

func (s *BrowserSessionService) readRedisStreamToken(ctx context.Context, sessionID uuid.UUID, tokenHash string, consume bool) (browserStreamTokenMeta, bool, error) {
	if s.coordinationRedisClient == nil {
		return browserStreamTokenMeta{}, false, nil
	}
	tokenKey := browserSessionStreamTokenKey(sessionID, tokenHash)
	var raw []byte
	var err error
	if consume {
		const script = `
if redis.call("GET", KEYS[2]) ~= ARGV[1] then
	return nil
end
local payload = redis.call("GET", KEYS[1])
if not payload then
	return nil
end
redis.call("DEL", KEYS[1])
redis.call("DEL", KEYS[2])
return payload
`
		var result any
		result, err = s.coordinationRedisClient.Eval(ctx, script, []string{tokenKey, browserSessionStreamCurrentKey(sessionID)}, tokenHash).Result()
		if err == nil {
			switch value := result.(type) {
			case string:
				raw = []byte(value)
			case []byte:
				raw = value
			default:
				err = fmt.Errorf("unexpected redis stream token payload type %T", result)
			}
		}
	} else {
		currentHash, currentErr := s.coordinationRedisClient.Get(ctx, browserSessionStreamCurrentKey(sessionID)).Result()
		if errors.Is(currentErr, redis.Nil) {
			return browserStreamTokenMeta{}, false, nil
		}
		if currentErr != nil {
			return browserStreamTokenMeta{}, false, currentErr
		}
		if currentHash != tokenHash {
			return browserStreamTokenMeta{}, false, nil
		}
		raw, err = s.coordinationRedisClient.Get(ctx, tokenKey).Bytes()
	}
	if errors.Is(err, redis.Nil) {
		return browserStreamTokenMeta{}, false, nil
	}
	if err != nil {
		return browserStreamTokenMeta{}, false, err
	}
	var meta browserStreamTokenMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return browserStreamTokenMeta{}, false, err
	}
	return meta, true, nil
}

func (s *BrowserSessionService) deleteRedisStreamToken(ctx context.Context, sessionID uuid.UUID) error {
	if s.coordinationRedisClient == nil {
		return nil
	}
	currentHash, err := s.coordinationRedisClient.Get(ctx, browserSessionStreamCurrentKey(sessionID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	keys := []string{browserSessionStreamCurrentKey(sessionID)}
	seen := map[string]struct{}{browserSessionStreamCurrentKey(sessionID): {}}
	if currentHash != "" {
		currentTokenKey := browserSessionStreamTokenKey(sessionID, currentHash)
		keys = append(keys, currentTokenKey)
		seen[currentTokenKey] = struct{}{}
	}
	iter := s.coordinationRedisClient.Scan(ctx, 0, browserSessionStreamTokenKeyPrefixFor(sessionID)+"*", 100).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		if _, ok := seen[key]; ok {
			continue
		}
		keys = append(keys, key)
		seen[key] = struct{}{}
	}
	if err := iter.Err(); err != nil {
		return err
	}
	var deleteErrs []error
	for _, key := range keys {
		if err := s.coordinationRedisClient.Del(ctx, key).Err(); err != nil {
			deleteErrs = append(deleteErrs, err)
		}
	}
	return errors.Join(deleteErrs...)
}

func (s *BrowserSessionService) hasCurrentStreamToken(ctx context.Context, session models.RemoteBrowserSession) (bool, error) {
	if s.coordinationRedisClient == nil {
		return session.ConnectTokenHash != "" && StreamTokenValidUntil(session).After(time.Now()), nil
	}
	currentHash, err := s.coordinationRedisClient.Get(ctx, browserSessionStreamCurrentKey(session.ID)).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if currentHash == "" {
		return false, nil
	}
	if err := s.coordinationRedisClient.Get(ctx, browserSessionStreamTokenKey(session.ID, currentHash)).Err(); errors.Is(err, redis.Nil) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func BrowserSessionStreamURL(sessionID uuid.UUID, token string) string {
	// Base64 RawURLEncoding is already URL-safe.
	// Escaping it again can lead to decoding issues in some proxy layers.
	streamBasePath := fmt.Sprintf(
		"api/user/dashboard/browser-sessions/%s/stream/%s",
		sessionID,
		token,
	)
	query := url.Values{
		"autoconnect": {"true"},
		"path":        {streamBasePath + "/websockify"},
		"resize":      {"scale"},
	}
	return fmt.Sprintf("/%s/vnc.html?%s", streamBasePath, query.Encode())
}

func StreamTokenExpiresAt(sessionExpiresAt time.Time, issuedAt ...time.Time) time.Time {
	now := time.Now()
	if len(issuedAt) > 0 && !issuedAt[0].IsZero() {
		now = issuedAt[0]
	}
	ttl := min(sessionExpiresAt.Sub(now), streamTokenMaxTTL)
	if ttl <= 0 {
		return now
	}
	return now.Add(ttl)
}

func StreamTokenValidUntil(session models.RemoteBrowserSession) time.Time {
	if !session.ConnectTokenExpiresAt.IsZero() {
		return session.ConnectTokenExpiresAt
	}
	return StreamTokenExpiresAt(session.ExpiresAt, session.CreatedAt)
}

func GenerateStreamToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	return token, HashStreamToken(token), nil
}

func HashStreamToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", hash)
}

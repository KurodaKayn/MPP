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
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/redis/go-redis/v9"
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
	expiresAt := streamTokenExpiresAt(sessionExpiresAt)
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return time.Time{}, ErrInvalidStreamToken
	}
	if s.redisClient == nil {
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
	const script = `
local old_hash = redis.call("GET", KEYS[1])
if old_hash and old_hash ~= ARGV[1] then
	redis.call("DEL", ARGV[4] .. old_hash)
end
redis.call("SET", KEYS[2], ARGV[2], "PX", ARGV[3])
redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[3])
return 1
`
	return expiresAt, s.redisClient.Eval(ctx, script, []string{
		browserSessionStreamCurrentKey(sessionID),
		browserSessionStreamTokenKey(sessionID, tokenHash),
	}, tokenHash, payload, ttl.Milliseconds(), browserSessionStreamTokenKeyPrefixFor(sessionID)).Err()
}

func (s *BrowserSessionService) readRedisStreamToken(ctx context.Context, sessionID uuid.UUID, tokenHash string, consume bool) (browserStreamTokenMeta, bool, error) {
	if s.redisClient == nil {
		return browserStreamTokenMeta{}, false, nil
	}
	tokenKey := browserSessionStreamTokenKey(sessionID, tokenHash)
	var raw []byte
	var err error
	if consume {
		const script = `
local payload = redis.call("GET", KEYS[1])
if not payload then
	return nil
end
redis.call("DEL", KEYS[1])
if redis.call("GET", KEYS[2]) == ARGV[1] then
	redis.call("DEL", KEYS[2])
end
return payload
`
		var result interface{}
		result, err = s.redisClient.Eval(ctx, script, []string{tokenKey, browserSessionStreamCurrentKey(sessionID)}, tokenHash).Result()
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
		raw, err = s.redisClient.Get(ctx, tokenKey).Bytes()
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
	if s.redisClient == nil {
		return nil
	}
	currentHash, err := s.redisClient.Get(ctx, browserSessionStreamCurrentKey(sessionID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	keys := []string{browserSessionStreamCurrentKey(sessionID)}
	if currentHash != "" {
		keys = append(keys, browserSessionStreamTokenKey(sessionID, currentHash))
	}
	return s.redisClient.Del(ctx, keys...).Err()
}

func (s *BrowserSessionService) hasCurrentStreamToken(ctx context.Context, session models.RemoteBrowserSession) (bool, error) {
	if s.redisClient == nil {
		return session.ConnectTokenHash != "" && streamTokenValidUntil(session).After(time.Now()), nil
	}
	currentHash, err := s.redisClient.Get(ctx, browserSessionStreamCurrentKey(session.ID)).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if currentHash == "" {
		return false, nil
	}
	if err := s.redisClient.Get(ctx, browserSessionStreamTokenKey(session.ID, currentHash)).Err(); errors.Is(err, redis.Nil) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func browserSessionStreamURL(sessionID uuid.UUID, token string) string {
	streamBasePath := fmt.Sprintf(
		"api/browser-stream/%s/%s",
		sessionID,
		url.PathEscape(token),
	)
	query := url.Values{
		"autoconnect": {"true"},
		"path":        {streamBasePath + "/websockify"},
		"resize":      {"scale"},
	}
	return fmt.Sprintf("/%s/vnc.html?%s", streamBasePath, query.Encode())
}

func streamTokenExpiresAt(sessionExpiresAt time.Time, issuedAt ...time.Time) time.Time {
	now := time.Now()
	if len(issuedAt) > 0 && !issuedAt[0].IsZero() {
		now = issuedAt[0]
	}
	ttl := sessionExpiresAt.Sub(now)
	if ttl > streamTokenMaxTTL {
		ttl = streamTokenMaxTTL
	}
	if ttl <= 0 {
		return now
	}
	return now.Add(ttl)
}

func streamTokenValidUntil(session models.RemoteBrowserSession) time.Time {
	if !session.ConnectTokenExpiresAt.IsZero() {
		return session.ConnectTokenExpiresAt
	}
	return streamTokenExpiresAt(session.ExpiresAt, session.CreatedAt)
}

func generateStreamToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	return token, hashStreamToken(token), nil
}

func hashStreamToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", hash)
}

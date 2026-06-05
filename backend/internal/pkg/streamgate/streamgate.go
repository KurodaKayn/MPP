package streamgate

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	"github.com/kurodakayn/mpp-backend/internal/pkg/envutil"
)

const (
	KindAI      = "ai"
	KindBrowser = "browser"

	defaultPrefix = "mpp:stream"
)

var (
	ErrLimitExceeded = errors.New("stream concurrency limit exceeded")
	ErrUnavailable   = errors.New("stream limiter unavailable")
)

type Config struct {
	Enabled bool
	Prefix  string
	AI      Limits
	Browser Limits
}

type Limits struct {
	User   int64
	Tenant int64
	IP     int64
	Global int64
	TTL    time.Duration
}

type AcquireRequest struct {
	Kind     string
	UserID   uuid.UUID
	TenantID string
	IP       string
	Resource string
}

type Lease struct {
	ID        string
	Kind      string
	ExpiresAt time.Time
	release   func(context.Context) error
}

func (l *Lease) Release(ctx context.Context) error {
	if l == nil || l.release == nil {
		return nil
	}
	return l.release(ctx)
}

type Limiter struct {
	config Config
	redis  *redis.Client
	memory *memoryStore
}

type LimitError struct {
	Scope string
}

func (e *LimitError) Error() string {
	if e == nil || e.Scope == "" {
		return ErrLimitExceeded.Error()
	}
	return fmt.Sprintf("%s: %s", ErrLimitExceeded, e.Scope)
}

func (e *LimitError) Unwrap() error {
	return ErrLimitExceeded
}

func New(redisClient *redis.Client, config Config) *Limiter {
	config = normalizeConfig(config)
	if !config.Enabled {
		return nil
	}
	limiter := &Limiter{config: config, redis: redisClient}
	if redisClient == nil {
		limiter.memory = newMemoryStore()
	}
	return limiter
}

func ConfigFromEnv() Config {
	return normalizeConfig(Config{
		Enabled: envutil.Bool("STREAM_GATE_ENABLED", true),
		Prefix:  envString("STREAM_GATE_KEY_PREFIX", defaultPrefix),
		AI: Limits{
			User:   envInt64("AI_STREAM_USER_CONNECTION_LIMIT", 2),
			Tenant: envInt64("AI_STREAM_TENANT_CONNECTION_LIMIT", 20),
			IP:     envInt64("AI_STREAM_IP_CONNECTION_LIMIT", 10),
			Global: envInt64("AI_STREAM_GLOBAL_CONNECTION_LIMIT", 200),
			TTL:    envutil.Duration("AI_STREAM_CONNECTION_TTL", 10*time.Minute),
		},
		Browser: Limits{
			User:   envInt64("BROWSER_STREAM_USER_CONNECTION_LIMIT", 1),
			Tenant: envInt64("BROWSER_STREAM_TENANT_CONNECTION_LIMIT", 10),
			IP:     envInt64("BROWSER_STREAM_IP_CONNECTION_LIMIT", 3),
			Global: envInt64("BROWSER_STREAM_GLOBAL_CONNECTION_LIMIT", 100),
			TTL:    envutil.Duration("BROWSER_STREAM_CONNECTION_TTL", 16*time.Minute),
		},
	})
}

func (l *Limiter) Acquire(ctx context.Context, req AcquireRequest) (*Lease, error) {
	if l == nil || !l.config.Enabled {
		return &Lease{}, nil
	}
	limits := l.limitsFor(req.Kind)
	if limits.TTL <= 0 {
		return &Lease{}, nil
	}
	connID, err := randomID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(limits.TTL)
	resource := normalizeResource(req.Resource)
	payload, err := json.Marshal(map[string]string{
		"conn_id":    connID,
		"kind":       req.Kind,
		"user_id":    req.UserID.String(),
		"tenant_id":  req.TenantID,
		"ip_hash":    hashIP(req.IP),
		"resource":   resource,
		"started_at": now.Format(time.RFC3339Nano),
		"expires_at": expiresAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, err
	}

	if l.redis == nil {
		if err := l.memory.acquire(req, limits, connID, now, expiresAt); err != nil {
			return nil, err
		}
		return &Lease{ID: connID, Kind: req.Kind, ExpiresAt: expiresAt, release: func(context.Context) error {
			l.memory.release(req, connID)
			return nil
		}}, nil
	}

	scope, err := l.acquireRedis(ctx, req, limits, connID, payload, now, expiresAt)
	if err != nil {
		return nil, err
	}
	if scope != "" {
		return nil, &LimitError{Scope: scope}
	}
	return &Lease{ID: connID, Kind: req.Kind, ExpiresAt: expiresAt, release: func(ctx context.Context) error {
		return l.releaseRedis(ctx, req, connID)
	}}, nil
}

func (l *Limiter) limitsFor(kind string) Limits {
	switch kind {
	case KindBrowser:
		return l.config.Browser
	default:
		return l.config.AI
	}
}

func normalizeConfig(config Config) Config {
	if strings.TrimSpace(config.Prefix) == "" {
		config.Prefix = defaultPrefix
	}
	config.AI = normalizeLimits(config.AI)
	config.Browser = normalizeLimits(config.Browser)
	return config
}

func normalizeLimits(limits Limits) Limits {
	if limits.User < 0 {
		limits.User = 0
	}
	if limits.Tenant < 0 {
		limits.Tenant = 0
	}
	if limits.IP < 0 {
		limits.IP = 0
	}
	if limits.Global < 0 {
		limits.Global = 0
	}
	return limits
}

const acquireScript = `
redis.call("ZREMRANGEBYSCORE", KEYS[2], "-inf", ARGV[1])
redis.call("ZREMRANGEBYSCORE", KEYS[3], "-inf", ARGV[1])
redis.call("ZREMRANGEBYSCORE", KEYS[4], "-inf", ARGV[1])
redis.call("ZREMRANGEBYSCORE", KEYS[5], "-inf", ARGV[1])
if tonumber(ARGV[5]) > 0 and redis.call("ZCARD", KEYS[2]) >= tonumber(ARGV[5]) then return {0, "user"} end
if tonumber(ARGV[6]) > 0 and redis.call("ZCARD", KEYS[3]) >= tonumber(ARGV[6]) then return {0, "tenant"} end
if tonumber(ARGV[7]) > 0 and redis.call("ZCARD", KEYS[4]) >= tonumber(ARGV[7]) then return {0, "ip"} end
if tonumber(ARGV[8]) > 0 and redis.call("ZCARD", KEYS[5]) >= tonumber(ARGV[8]) then return {0, "global"} end
redis.call("SET", KEYS[1], ARGV[4], "PX", ARGV[3])
redis.call("ZADD", KEYS[2], ARGV[2], ARGV[9])
redis.call("ZADD", KEYS[3], ARGV[2], ARGV[9])
redis.call("ZADD", KEYS[4], ARGV[2], ARGV[9])
redis.call("ZADD", KEYS[5], ARGV[2], ARGV[9])
return {1, ""}
`

func (l *Limiter) acquireRedis(ctx context.Context, req AcquireRequest, limits Limits, connID string, payload []byte, now time.Time, expiresAt time.Time) (string, error) {
	result, err := l.redis.Eval(ctx, acquireScript, l.keys(req, connID),
		now.UnixMilli(),
		expiresAt.UnixMilli(),
		limits.TTL.Milliseconds(),
		string(payload),
		limits.User,
		limits.Tenant,
		limits.IP,
		limits.Global,
		connID,
	).Result()
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	values, ok := result.([]any)
	if !ok || len(values) != 2 {
		log.Printf("streamgate: unexpected redis acquire response: %#v", result)
		return "", fmt.Errorf("%w: unexpected redis response", ErrUnavailable)
	}
	accepted, ok := redisInt64(values[0])
	if !ok {
		log.Printf("streamgate: unexpected redis acquire accepted flag: %#v", values[0])
		return "", fmt.Errorf("%w: unexpected redis accepted flag", ErrUnavailable)
	}
	if accepted == 1 {
		return "", nil
	}
	scope, _ := values[1].(string)
	if scope == "" {
		scope = "unknown"
	}
	return scope, nil
}

func (l *Limiter) releaseRedis(ctx context.Context, req AcquireRequest, connID string) error {
	keys := l.keys(req, connID)
	if err := l.redis.Del(ctx, keys[0]).Err(); err != nil {
		return err
	}
	for _, key := range keys[1:] {
		if err := l.redis.ZRem(ctx, key, connID).Err(); err != nil {
			return err
		}
	}
	return nil
}

func (l *Limiter) keys(req AcquireRequest, connID string) []string {
	prefix := strings.TrimRight(l.config.Prefix, ":")
	kind := keyPart(req.Kind)
	userID := keyPart(req.UserID.String())
	tenantID := keyPart(req.TenantID)
	ipHash := keyPart(hashIP(req.IP))
	return []string{
		prefix + ":conn:" + connID,
		prefix + ":" + kind + ":user:" + userID,
		prefix + ":" + kind + ":tenant:" + tenantID,
		prefix + ":" + kind + ":ip:" + ipHash,
		prefix + ":" + kind + ":global",
	}
}

func SendLimitError(c echo.Context, err error) error {
	var limitErr *LimitError
	if errors.As(err, &limitErr) {
		return c.JSON(http.StatusTooManyRequests, map[string]any{
			"error": map[string]string{
				"code":    "stream_concurrency_exceeded",
				"message": "stream concurrency limit exceeded",
				"scope":   limitErr.Scope,
			},
		})
	}
	if errors.Is(err, ErrUnavailable) {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{
			"error": map[string]string{
				"code":    "stream_limiter_unavailable",
				"message": "stream limiter is unavailable",
			},
		})
	}
	return nil
}

func ClientIP(c echo.Context) string {
	if c == nil || c.Request() == nil {
		return ""
	}
	if ip := strings.TrimSpace(c.RealIP()); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(c.Request().RemoteAddr)
	if err == nil {
		return host
	}
	return c.Request().RemoteAddr
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func hashIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		ip = "unknown"
	}
	sum := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(sum[:])
}

func keyPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == ':':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "unknown"
	}
	return result
}

func redisInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case string:
		parsed, err := strconv.ParseInt(typed, 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func envString(name string, fallback string) string {
	if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
		return raw
	}
	return fallback
}

func envInt64(name string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func normalizeResource(resource string) string {
	resource = strings.TrimSpace(resource)
	if resource == "" {
		return "unknown"
	}
	return resource
}

type memoryStore struct {
	mu    sync.Mutex
	conns map[string]memoryConn
}

type memoryConn struct {
	req       AcquireRequest
	ipHash    string
	expiresAt time.Time
}

func newMemoryStore() *memoryStore {
	return &memoryStore{conns: map[string]memoryConn{}}
}

func (s *memoryStore) acquire(req AcquireRequest, limits Limits, connID string, now time.Time, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune(now)
	reqIPHash := hashIP(req.IP)
	counts := map[string]int64{}
	for _, conn := range s.conns {
		if conn.req.Kind != req.Kind {
			continue
		}
		counts["global"]++
		if conn.req.UserID == req.UserID {
			counts["user"]++
		}
		if conn.req.TenantID == req.TenantID {
			counts["tenant"]++
		}
		if conn.ipHash == reqIPHash {
			counts["ip"]++
		}
	}
	if limits.User > 0 && counts["user"] >= limits.User {
		return &LimitError{Scope: "user"}
	}
	if limits.Tenant > 0 && counts["tenant"] >= limits.Tenant {
		return &LimitError{Scope: "tenant"}
	}
	if limits.IP > 0 && counts["ip"] >= limits.IP {
		return &LimitError{Scope: "ip"}
	}
	if limits.Global > 0 && counts["global"] >= limits.Global {
		return &LimitError{Scope: "global"}
	}
	s.conns[connID] = memoryConn{req: req, ipHash: reqIPHash, expiresAt: expiresAt}
	return nil
}

func (s *memoryStore) release(_ AcquireRequest, connID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, connID)
}

func (s *memoryStore) prune(now time.Time) {
	for connID, conn := range s.conns {
		if !conn.expiresAt.After(now) {
			delete(s.conns, connID)
		}
	}
}

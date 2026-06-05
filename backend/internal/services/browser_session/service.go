package browsersession

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
)

var (
	ErrActiveSessionExists  = errors.New("an active session already exists for this platform")
	ErrPlatformNotSupported = errors.New("platform does not support remote browser sessions")
	ErrSessionNotFound      = errors.New("session not found")
	ErrSessionForbidden     = errors.New("session does not belong to the authenticated user")
	ErrSessionGone          = errors.New("session has expired")
	ErrInvalidStreamToken   = errors.New("invalid or expired stream token")
	ErrStreamTokenGone      = errors.New("stream token has expired or already been consumed")
	ErrSessionNotReady      = errors.New("session is not ready for capture")
	ErrLoginNotDetected     = errors.New("login not detected")
	ErrUserQuotaExceeded    = errors.New("browser session user concurrency quota exceeded")
	ErrTenantQuotaExceeded  = errors.New("browser session tenant concurrency quota exceeded")
	ErrWorkerPoolExhausted  = errors.New("browser worker session pool exhausted")
)

const (
	pendingSessionStaleAfter = 2 * time.Minute
	browserSessionTTL        = 15 * time.Minute
	browserSessionRedisGrace = 1 * time.Minute
	streamTokenMaxTTL        = 5 * time.Minute

	browserSessionActiveKeyPrefix       = "mpp:browser:active:"
	browserSessionKeyPrefix             = "mpp:browser:session:"
	browserSessionStreamTokenPrefix     = "mpp:browser:stream-token:" //nolint:gosec // Redis key prefix, not a token value.
	browserSessionStreamCurrentPrefix   = "mpp:browser:stream-current:"
	browserSessionWorkerHeartbeatPrefix = "mpp:browser:worker-heartbeat:"
	browserSessionQuotaUserPrefix       = "mpp:browser:quota:user:"
	browserSessionQuotaTenantPrefix     = "mpp:browser:quota:tenant:"
	browserSessionCleanupKey            = "mpp:browser:cleanup"

	browserSessionDefaultTenantID = "default"

	browserSessionUserConcurrencyLimitEnv   = "BROWSER_SESSION_USER_CONCURRENCY_LIMIT"
	browserSessionTenantConcurrencyLimitEnv = "BROWSER_SESSION_TENANT_CONCURRENCY_LIMIT"

	defaultBrowserSessionUserConcurrencyLimit   int64 = 2
	defaultBrowserSessionTenantConcurrencyLimit int64 = 10
)

type BrowserSessionQuotaConfig struct {
	UserConcurrencyLimit   int64
	TenantConcurrencyLimit int64
}

type BrowserSessionService struct {
	db           *gorm.DB
	workerClient publisher.BrowserWorkerClient
	cookieStore  *publisher.CookieStore
	adapters     map[string]publisher.RemoteBrowserPlatformAdapter
	redisClient  *redis.Client
	quotaConfig  BrowserSessionQuotaConfig
}

func NewBrowserSessionService(db *gorm.DB, worker publisher.BrowserWorkerClient, store *publisher.CookieStore) *BrowserSessionService {
	s := &BrowserSessionService{
		db:           db,
		workerClient: worker,
		cookieStore:  store,
		adapters:     make(map[string]publisher.RemoteBrowserPlatformAdapter),
		quotaConfig:  BrowserSessionQuotaConfigFromEnv(),
	}
	// Register adapters
	s.RegisterAdapter(&publisher.DouyinAdapter{})
	s.RegisterAdapter(&publisher.ZhihuAdapter{})
	return s
}

func (s *BrowserSessionService) WithContext(ctx context.Context) *BrowserSessionService {
	if ctx == nil {
		return s
	}
	scoped := *s
	scoped.db = s.db.WithContext(ctx)
	if s.cookieStore != nil {
		scoped.cookieStore = s.cookieStore.WithContext(ctx)
	}
	return &scoped
}

func (s *BrowserSessionService) RegisterAdapter(a publisher.RemoteBrowserPlatformAdapter) {
	s.adapters[a.Platform()] = a
}

func (s *BrowserSessionService) RegisterSession(ctx context.Context, session *models.RemoteBrowserSession, tokenHash string) error {
	if err := s.db.WithContext(ctx).Create(session).Error; err != nil {
		return err
	}

	if s.redisClient != nil {
		// Register in Redis live sessions
		if err := s.saveRedisLiveSession(ctx, browserSessionLiveState{
			SessionID:         session.ID,
			UserID:            session.UserID,
			Platform:          session.Platform,
			Status:            session.Status,
			WorkerSessionRef:  session.WorkerSessionRef,
			ContainerID:       session.ContainerID,
			CDPEndpointRef:    session.CDPEndpointRef,
			StreamEndpointRef: session.StreamEndpointRef,
			CreatedAt:         session.CreatedAt,
			ExpiresAt:         session.ExpiresAt,
		}); err != nil {
			return err
		}

		// Register token in Redis
		_, err := s.rotateRedisStreamToken(ctx, session.ID, session.UserID, session.Platform, tokenHash, session.ExpiresAt)
		return err
	}

	return nil
}

func (s *BrowserSessionService) UseRedis(client *redis.Client) {
	if client == nil {
		return
	}
	s.redisClient = client
}

func (s *BrowserSessionService) UseQuotaConfig(config BrowserSessionQuotaConfig) {
	s.quotaConfig = normalizeBrowserSessionQuotaConfig(config)
}

func DefaultBrowserSessionQuotaConfig() BrowserSessionQuotaConfig {
	return BrowserSessionQuotaConfig{
		UserConcurrencyLimit:   defaultBrowserSessionUserConcurrencyLimit,
		TenantConcurrencyLimit: defaultBrowserSessionTenantConcurrencyLimit,
	}
}

func BrowserSessionQuotaConfigFromEnv() BrowserSessionQuotaConfig {
	defaults := DefaultBrowserSessionQuotaConfig()
	return BrowserSessionQuotaConfig{
		UserConcurrencyLimit:   int64FromEnv(browserSessionUserConcurrencyLimitEnv, defaults.UserConcurrencyLimit),
		TenantConcurrencyLimit: int64FromEnv(browserSessionTenantConcurrencyLimitEnv, defaults.TenantConcurrencyLimit),
	}
}

func normalizeBrowserSessionQuotaConfig(config BrowserSessionQuotaConfig) BrowserSessionQuotaConfig {
	if config.UserConcurrencyLimit < 0 {
		config.UserConcurrencyLimit = 0
	}
	if config.TenantConcurrencyLimit < 0 {
		config.TenantConcurrencyLimit = 0
	}
	return config
}

func normalizeBrowserSessionTenantID(tenantID string) string {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return browserSessionDefaultTenantID
	}
	return tenantID
}

func int64FromEnv(name string, fallback int64) int64 {
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

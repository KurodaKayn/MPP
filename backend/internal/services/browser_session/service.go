package browsersession

import (
	"errors"
	"time"

	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
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
)

const (
	pendingSessionStaleAfter = 2 * time.Minute
	browserSessionTTL        = 15 * time.Minute
	browserSessionRedisGrace = 1 * time.Minute
	streamTokenMaxTTL        = 5 * time.Minute

	browserSessionActiveKeyPrefix       = "mpp:browser:active:"
	browserSessionKeyPrefix             = "mpp:browser:session:"
	browserSessionStreamTokenPrefix     = "mpp:browser:stream-token:"
	browserSessionStreamCurrentPrefix   = "mpp:browser:stream-current:"
	browserSessionWorkerHeartbeatPrefix = "mpp:browser:worker-heartbeat:"
	browserSessionCleanupKey            = "mpp:browser:cleanup"
)

type BrowserSessionService struct {
	db           *gorm.DB
	workerClient publisher.BrowserWorkerClient
	cookieStore  *publisher.CookieStore
	adapters     map[string]publisher.RemoteBrowserPlatformAdapter
	redisClient  *redis.Client
}

func NewBrowserSessionService(db *gorm.DB, worker publisher.BrowserWorkerClient, store *publisher.CookieStore) *BrowserSessionService {
	s := &BrowserSessionService{
		db:           db,
		workerClient: worker,
		cookieStore:  store,
		adapters:     make(map[string]publisher.RemoteBrowserPlatformAdapter),
	}
	// Register adapters
	s.RegisterAdapter(&publisher.DouyinAdapter{})
	s.RegisterAdapter(&publisher.ZhihuAdapter{})
	return s
}

func (s *BrowserSessionService) RegisterAdapter(a publisher.RemoteBrowserPlatformAdapter) {
	s.adapters[a.Platform()] = a
}

func (s *BrowserSessionService) UseRedis(client *redis.Client) {
	if client == nil {
		return
	}
	s.redisClient = client
}

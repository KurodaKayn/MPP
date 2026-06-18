package app

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/observability"
	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
	objectstorager2 "github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage/r2"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"github.com/kurodakayn/mpp-backend/internal/redisclient"
	"github.com/kurodakayn/mpp-backend/internal/services/archive"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	collabdoc "github.com/kurodakayn/mpp-backend/internal/services/collabdoc"
	dashboardsvc "github.com/kurodakayn/mpp-backend/internal/services/dashboard"
	"github.com/kurodakayn/mpp-backend/internal/services/email"
)

type RuntimeMode string

const (
	RuntimeModeAPI           RuntimeMode = "api"
	RuntimeModePublishWorker RuntimeMode = "publish-worker"
)

type RuntimeWiringConfig struct {
	Mode          RuntimeMode
	RuntimeConfig RuntimeConfig
	JWTSecret     string
	SQLDB         *gorm.DB
	DBRouter      *db.Router
}

type Runtime struct {
	Config                 RuntimeConfig
	JWTSigningKey          []byte
	MockLogin              bool
	RedisClient            *redis.Client
	RedisCoordination      *redis.Client
	RedisCache             *redis.Client
	RedisQueue             *redis.Client
	RedisSessionContinuity *redis.Client
	ObservabilitySuite     *observability.Suite
	DashboardService       *dashboardsvc.DashboardService
	CollabDocumentService  *collabdoc.Service
	BrowserSessionService  *browsersession.BrowserSessionService
	ObjectStorageConfig    objectstorage.Config
	ObjectStorageClient    objectstorage.Client
	ArchiveConfig          archive.Config
	BaseEmailService       email.EmailService
	EmailService           email.EmailService

	db                *gorm.DB
	asyncEmailService *email.AsyncEmailService
	workerWG          sync.WaitGroup
}

type RuntimeWorkerErrors struct {
	Email     <-chan error
	Publish   <-chan error
	ReadModel <-chan error
}

func NewRuntime(ctx context.Context, config RuntimeWiringConfig) (*Runtime, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if config.SQLDB == nil {
		return nil, errors.New("runtime wiring requires a database")
	}
	mode := config.Mode
	if mode == "" {
		mode = RuntimeModeAPI
	}

	observabilitySuite := observability.New(serviceNameForRuntime(mode, config.RuntimeConfig))
	dashboardService := dashboardsvc.NewDashboardServiceWithRouter(config.SQLDB, config.DBRouter)
	dashboardService.SetPublishJobObserver(observabilitySuite.PublishJobObserver())

	objectStorageConfig, objectStorageClient, err := objectStorageClientFromEnv()
	if err != nil {
		return nil, err
	}
	if objectStorageClient != nil {
		dashboardService.UseObjectStorage(objectStorageClient, objectStorageConfig)
	}

	archiveConfig, err := archive.ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	if archiveConfig.Enabled && objectStorageClient == nil {
		return nil, errors.New("EVENT_ARCHIVE_ENABLED requires OBJECT_STORAGE_PROVIDER=r2")
	}

	runtime := &Runtime{
		Config:              config.RuntimeConfig,
		JWTSigningKey:       []byte(strings.TrimSpace(config.JWTSecret)),
		MockLogin:           MockLoginEnabled(),
		ObservabilitySuite:  observabilitySuite,
		DashboardService:    dashboardService,
		ObjectStorageConfig: objectStorageConfig,
		ObjectStorageClient: objectStorageClient,
		ArchiveConfig:       archiveConfig,
		db:                  config.SQLDB,
	}
	runtime.wireCollabDocumentService(config.JWTSecret)

	redisClients, err := redisclient.NewClientSetFromEnv(ctx)
	if errors.Is(err, redisclient.ErrNotConfigured) {
		if redisRequiredForRuntime(mode, config.RuntimeConfig) {
			return nil, errors.New(missingRedisMessage(mode))
		}
	} else if err != nil {
		return nil, err
	} else {
		runtime.RedisClient = redisClients.Default
		runtime.RedisCoordination = redisClients.Coordination
		runtime.RedisCache = redisClients.Cache
		runtime.RedisQueue = redisClients.Queue
		runtime.RedisSessionContinuity = redisClients.Session

		dashboardService.AccountSettings.UseRedisStateStore(redisClients.Session)
		dashboardService.UseRedisCache(redisClients.Cache)
		dashboardService.UseRedisQueue(redisClients.Queue)
		dashboardService.Publisher.UseRedisCoordination(redisClients.Coordination)
	}

	workerClient := NewBrowserWorkerClientFromEnv()
	browserSessionService := browsersession.NewBrowserSessionServiceWithRouter(config.SQLDB, workerClient, publisher.NewCookieStore(config.SQLDB), config.DBRouter)
	browserSessionService.UseDashboardAccountCacheInvalidator(dashboardService.AccountSettings)
	if runtime.RedisCoordination != nil {
		browserSessionService.UseRedisCoordination(runtime.RedisCoordination)
	}
	if runtime.RedisSessionContinuity != nil {
		browserSessionService.UseRedisContinuity(runtime.RedisSessionContinuity)
	}
	dashboardService.SetBrowserWorkerClient(workerClient)
	dashboardService.SetBrowserSessionService(browserSessionService)
	runtime.BrowserSessionService = browserSessionService

	baseEmailService, err := NewBaseEmailServiceFromEnv()
	if err != nil {
		return nil, err
	}
	runtime.BaseEmailService = baseEmailService
	runtime.EmailService = baseEmailService
	if runtime.RedisQueue != nil {
		asyncEmailService := email.NewAsyncEmailService(runtime.RedisQueue)
		runtime.asyncEmailService = asyncEmailService
		runtime.EmailService = asyncEmailService
	}

	return runtime, nil
}

func (r *Runtime) StartAPIWorkers(ctx context.Context) RuntimeWorkerErrors {
	if r == nil || !r.Config.RunsWorkers() {
		return RuntimeWorkerErrors{}
	}

	workerErrors := RuntimeWorkerErrors{}
	r.startArchiveWorker(ctx)
	if r.RedisQueue == nil && r.RedisSessionContinuity == nil {
		return workerErrors
	}

	if r.RedisQueue != nil {
		r.DashboardService.StartPublishWorker(ctx)
	}
	if r.BrowserSessionService != nil {
		r.BrowserSessionService.StartCleanupWorker(ctx)
	}
	workerErrors.Email = r.startEmailWorker(ctx)
	if r.RedisQueue != nil {
		workerErrors.ReadModel = r.DashboardService.StartDashboardReadModelRebuildWorkerWithErrors(ctx)
	}
	return workerErrors
}

func (r *Runtime) StartPublishWorkerMode(ctx context.Context) RuntimeWorkerErrors {
	if r == nil {
		return RuntimeWorkerErrors{}
	}

	workerErrors := RuntimeWorkerErrors{
		Publish:   r.DashboardService.StartPublishWorkerWithErrors(ctx),
		ReadModel: r.DashboardService.StartDashboardReadModelRebuildWorkerWithErrors(ctx),
		Email:     r.startEmailWorker(ctx),
	}
	if r.BrowserSessionService != nil {
		r.BrowserSessionService.StartCleanupWorker(ctx)
	}
	r.startArchiveWorker(ctx)
	return workerErrors
}

func (r *Runtime) WaitWorkers() {
	if r == nil {
		return
	}
	r.workerWG.Wait()
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	clients := []*redis.Client{
		r.RedisClient,
		r.RedisCoordination,
		r.RedisCache,
		r.RedisQueue,
		r.RedisSessionContinuity,
	}
	seen := map[*redis.Client]struct{}{}
	var firstErr error
	for _, client := range clients {
		if client == nil {
			continue
		}
		if _, ok := seen[client]; ok {
			continue
		}
		seen[client] = struct{}{}
		if err := client.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (r *Runtime) wireCollabDocumentService(jwtSecret string) {
	secret := strings.TrimSpace(jwtSecret)
	if secret == "" {
		return
	}

	collabDocumentService := collabdoc.NewService(r.db)
	collabSecret := []byte(CollabTokenSecret(secret))
	collabDocumentService.UseSessionConfig(collabdoc.SessionConfig{
		TokenSecret:      collabSecret,
		WebsocketURLBase: CollabWebsocketURLBase(),
	})
	collabDocumentService.UseProjectDocumentInitializer(
		collabdoc.NewHTTPProjectDocumentInitializer(CollabInternalURL(), collabSecret, nil),
	)
	r.CollabDocumentService = collabDocumentService
	r.DashboardService.SetCollabDocumentService(collabDocumentService)
}

func (r *Runtime) startArchiveWorker(ctx context.Context) {
	if r == nil || !r.ArchiveConfig.Enabled {
		return
	}
	archive.NewWorker(r.db, r.ObjectStorageClient, r.ArchiveConfig).Start(ctx)
}

func (r *Runtime) startEmailWorker(ctx context.Context) <-chan error {
	if r == nil || r.asyncEmailService == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	workerErrors := make(chan error, 1)
	r.workerWG.Go(func() {
		if err := r.asyncEmailService.StartWorker(ctx, r.BaseEmailService); err != nil && ctx.Err() == nil {
			select {
			case workerErrors <- err:
			default:
				log.Printf("email worker stopped with error: %v", err)
			}
		}
	})
	return workerErrors
}

func redisRequiredForRuntime(mode RuntimeMode, config RuntimeConfig) bool {
	return mode == RuntimeModePublishWorker || config.RequireRedis
}

func missingRedisMessage(mode RuntimeMode) string {
	if mode == RuntimeModePublishWorker {
		return "REDIS_ADDR must be set for publish-worker"
	}
	return "REDIS_ADDR must be set when BACKEND_REQUIRE_REDIS is enabled"
}

func serviceNameForRuntime(mode RuntimeMode, config RuntimeConfig) string {
	if mode == RuntimeModePublishWorker {
		return PublishWorkerServiceName
	}
	return config.ServiceName()
}

func objectStorageClientFromEnv() (objectstorage.Config, objectstorage.Client, error) {
	config, err := objectstorage.ConfigFromEnv()
	if err != nil || !config.Enabled {
		return config, nil, err
	}
	client, err := objectstorager2.NewClient(config)
	return config, client, err
}

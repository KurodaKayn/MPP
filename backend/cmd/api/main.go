package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/handlers"
	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
	objectstorager2 "github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage/r2"
	"github.com/kurodakayn/mpp-backend/internal/pkg/streamgate"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"github.com/kurodakayn/mpp-backend/internal/redisclient"
	"github.com/kurodakayn/mpp-backend/internal/services"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	"github.com/kurodakayn/mpp-backend/internal/services/email"
)

const shutdownTimeout = 15 * time.Second

func main() {
	rootCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	// Load .env file if it exists
	_ = godotenv.Load()

	runtimeConfig, err := backendRuntimeConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	jwtSecret, err := requiredEnv(jwtSecretEnv)
	if err != nil {
		log.Fatal(err)
	}
	jwtSigningKey := []byte(jwtSecret)

	// Initialize Database
	db.InitDB()

	// Initialize Services and Handlers
	dashboardService := services.NewDashboardService(db.DB)
	objectStorageConfig, err := objectstorage.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	if objectStorageConfig.Enabled {
		objectStorageClient, err := objectstorager2.NewClient(objectStorageConfig)
		if err != nil {
			log.Fatal(err)
		}
		dashboardService.UseObjectStorage(objectStorageClient, objectStorageConfig)
	}
	collabDocumentService := services.NewCollabDocumentService(db.DB)
	collabSecret := []byte(collabTokenSecret(jwtSecret))
	collabDocumentService.UseSessionConfig(services.CollabDocumentSessionConfig{
		TokenSecret:      collabSecret,
		WebsocketURLBase: collabWebsocketURLBase(),
	})
	collabDocumentService.UseProjectDocumentInitializer(
		services.NewHTTPProjectDocumentInitializer(collabInternalURL(), collabSecret, nil),
	)
	dashboardService.SetCollabDocumentService(collabDocumentService)
	redisClient, err := redisclient.NewFromEnv(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	if runtimeConfig.requireRedis && redisClient == nil {
		log.Fatal("REDIS_ADDR must be set when BACKEND_REQUIRE_REDIS is enabled")
	}

	// Remote Browser Session (New)
	var workerClient publisher.BrowserWorkerClient
	workerURL := os.Getenv("BROWSER_WORKER_URL")
	if workerURL != "" {
		workerClient = publisher.NewHttpBrowserWorkerClient(workerURL)
	} else {
		workerClient = publisher.NewMockBrowserWorkerClient()
	}
	browserSessionService := browsersession.NewBrowserSessionService(db.DB, workerClient, publisher.NewCookieStore(db.DB))
	dashboardService.SetBrowserWorkerClient(workerClient)
	dashboardService.SetBrowserSessionService(browserSessionService)

	if redisClient != nil {
		dashboardService.UseRedis(redisClient)
		if runtimeConfig.runsWorkers() {
			dashboardService.StartPublishWorker(rootCtx)
		}
	}

	// Email Service
	var baseEmailService email.EmailService
	smtpHost := os.Getenv("SMTP_HOST")
	if smtpHost != "" {
		smtpPort := 587
		if rawPort := strings.TrimSpace(os.Getenv("SMTP_PORT")); rawPort != "" {
			parsedPort, err := strconv.Atoi(rawPort)
			if err != nil || parsedPort <= 0 {
				log.Fatalf("invalid SMTP_PORT: %s", rawPort)
			}
			smtpPort = parsedPort
		}
		smtpFrom := strings.TrimSpace(os.Getenv("SMTP_FROM"))
		smtpPassword := strings.TrimSpace(os.Getenv("SMTP_PASSWORD"))
		if smtpFrom == "" || smtpPassword == "" {
			log.Fatal("SMTP_FROM and SMTP_PASSWORD must be set when SMTP_HOST is set")
		}
		baseEmailService = email.NewSMTPEmailService(
			smtpHost,
			smtpPort,
			smtpFrom,
			smtpPassword,
		)
	} else {
		baseEmailService = &email.MockEmailService{}
	}

	emailService := baseEmailService
	workerErrors := make(chan error, 1)
	var workerWG sync.WaitGroup
	if redisClient != nil {
		asyncEmailService := email.NewAsyncEmailService(redisClient)
		emailService = asyncEmailService
		if runtimeConfig.runsWorkers() {
			workerWG.Add(1)
			go func() {
				defer workerWG.Done()
				if err := asyncEmailService.StartWorker(rootCtx, baseEmailService); err != nil {
					select {
					case workerErrors <- err:
					default:
						log.Printf("email worker stopped with error: %v", err)
					}
				}
			}()
		}
	}

	adminDashboardHandler := handlers.NewDashboardHandler(dashboardService)
	userDashboardHandler := handlers.NewUserDashboardHandler(dashboardService)
	collabDocumentHandler := handlers.NewCollabDocumentHandler(collabDocumentService)
	userDashboardHandler.UseAIContentEditor(services.NewAIServiceClientFromEnv())
	streamLimiter := streamgate.New(redisClient, streamgate.ConfigFromEnv())
	userDashboardHandler.UseStreamLimiter(streamLimiter)
	mockLogin := mockLoginEnabled()
	authHandler := handlers.NewAuthHandler(db.DB, redisClient, emailService, jwtSigningKey)
	authHandler.SetUsernameLoginEnabled(mockLogin)

	if redisClient != nil {
		browserSessionService.UseRedis(redisClient)
		if runtimeConfig.runsWorkers() {
			browserSessionService.StartCleanupWorker(rootCtx)
		}
	}
	browserSessionHandler := handlers.NewBrowserSessionHandler(browserSessionService)
	browserSessionHandler.UseStreamLimiter(streamLimiter)

	ready := atomic.Bool{}
	ready.Store(true)

	server, err := newServer(serverConfig{
		runtimeConfig: runtimeConfig,
		jwtSigningKey: jwtSigningKey,
		redisClient:   redisClient,
		mockLogin:     mockLogin,
		ready:         &ready,
		sqlDB:         db.DB,
	}, serverHandlers{
		adminDashboard: adminDashboardHandler,
		userDashboard:  userDashboardHandler,
		auth:           authHandler,
		browserSession: browserSessionHandler,
		collabDocument: collabDocumentHandler,
	})
	if err != nil {
		log.Fatal(err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Start(":" + port)
	}()

	select {
	case err := <-serverErrors:
		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	case err := <-workerErrors:
		log.Fatalf("email worker stopped: %v", err)
	case <-rootCtx.Done():
		ready.Store(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Fatal(err)
		}
		workerWG.Wait()
		if redisClient != nil {
			_ = redisClient.Close()
		}
	}
}

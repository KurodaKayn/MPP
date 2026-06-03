package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/handlers"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"github.com/kurodakayn/mpp-backend/internal/redisclient"
	"github.com/kurodakayn/mpp-backend/internal/services"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
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

	adminDashboardHandler := handlers.NewDashboardHandler(dashboardService)
	userDashboardHandler := handlers.NewUserDashboardHandler(dashboardService)
	userDashboardHandler.UseAIContentEditor(services.NewAIServiceClientFromEnv())
	mockLogin := mockLoginEnabled()
	authHandler := handlers.NewAuthHandler(db.DB, jwtSigningKey)
	authHandler.SetUsernameLoginEnabled(mockLogin)

	if redisClient != nil {
		browserSessionService.UseRedis(redisClient)
		if runtimeConfig.runsWorkers() {
			browserSessionService.StartCleanupWorker(rootCtx)
		}
	}
	browserSessionHandler := handlers.NewBrowserSessionHandler(browserSessionService)

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
	case <-rootCtx.Done():
		ready.Store(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Fatal(err)
		}
		if redisClient != nil {
			_ = redisClient.Close()
		}
	}
}

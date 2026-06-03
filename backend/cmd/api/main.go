package main

import (
	"context"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/handlers"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"github.com/kurodakayn/mpp-backend/internal/redisclient"
	"github.com/kurodakayn/mpp-backend/internal/services"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
)

func main() {
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
		defer redisClient.Close()
		dashboardService.UseRedis(redisClient)
		if runtimeConfig.runsWorkers() {
			dashboardService.StartPublishWorker(context.Background())
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
			browserSessionService.StartCleanupWorker(context.Background())
		}
	}
	browserSessionHandler := handlers.NewBrowserSessionHandler(browserSessionService)

	server, err := newServer(serverConfig{
		runtimeConfig: runtimeConfig,
		jwtSigningKey: jwtSigningKey,
		redisClient:   redisClient,
		mockLogin:     mockLogin,
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

	// Start server
	server.Logger.Fatal(server.Start(":" + port))
}

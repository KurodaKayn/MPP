package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/kurodakayn/mpp-backend/internal/app"
	"github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/handlers"
	"github.com/kurodakayn/mpp-backend/internal/pkg/streamgate"
	aisvc "github.com/kurodakayn/mpp-backend/internal/services/ai"
)

const shutdownTimeout = 15 * time.Second

func main() {
	rootCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	// Load .env file if it exists
	_ = godotenv.Load()

	runtimeConfig, err := app.RuntimeConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	jwtSecret, err := app.RequiredEnv(app.JWTSecretEnv)
	if err != nil {
		log.Fatal(err)
	}

	// Initialize Database
	db.InitDB()

	runtime, err := app.NewRuntime(rootCtx, app.RuntimeWiringConfig{
		Mode:          app.RuntimeModeAPI,
		RuntimeConfig: runtimeConfig,
		JWTSecret:     jwtSecret,
		SQLDB:         db.DB,
		DBRouter:      db.DefaultRouter,
	})
	if err != nil {
		log.Fatal(err)
	}
	jwtSigningKey := runtime.JWTSigningKey

	workerErrors := runtime.StartAPIWorkers(rootCtx)

	adminDashboardHandler := handlers.NewDashboardHandler(runtime.DashboardService)
	userDashboardHandler := handlers.NewUserDashboardHandler(runtime.DashboardService)
	collabDocumentHandler := handlers.NewCollabDocumentHandler(runtime.CollabDocumentService)
	aiServiceClient := aisvc.NewAIServiceClientFromEnv()
	userDashboardHandler.UseAIContentEditor(aiServiceClient)
	userDashboardHandler.UseAIDraftingService(aisvc.NewDraftingService(db.DB))
	userDashboardHandler.UseAIGrowthOptimizationService(aisvc.NewGrowthOptimizationService(db.DB, aiServiceClient))
	userDashboardHandler.UseQuotaService(aisvc.NewQuotaService(db.DB))
	streamLimiter := streamgate.New(runtime.RedisClient, streamgate.ConfigFromEnv())
	userDashboardHandler.UseStreamLimiter(streamLimiter)
	authHandler := handlers.NewAuthHandler(db.DB, runtime.RedisClient, runtime.EmailService, jwtSigningKey)
	authHandler.SetUsernameLoginEnabled(runtime.MockLogin)

	browserSessionHandler := handlers.NewBrowserSessionHandler(runtime.BrowserSessionService)
	browserSessionHandler.UseStreamLimiter(streamLimiter)

	ready := atomic.Bool{}
	ready.Store(true)

	server, err := newServer(serverConfig{
		runtimeConfig:      runtimeConfig,
		jwtSigningKey:      jwtSigningKey,
		redisClient:        runtime.RedisClient,
		mockLogin:          runtime.MockLogin,
		ready:              &ready,
		sqlDB:              db.DB,
		dbRouter:           db.DefaultRouter,
		observabilitySuite: runtime.ObservabilitySuite,
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

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Start(":" + app.PortFromEnv())
	}()

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	case err := <-workerErrors.Email:
		log.Fatalf("email worker stopped: %v", err)
	case err := <-workerErrors.ReadModel:
		ready.Store(false)
		if err != nil {
			log.Fatalf("dashboard read model rebuild worker stopped: %v", err)
		}
	case <-rootCtx.Done():
		ready.Store(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Fatal(err)
		}
		runtime.WaitWorkers()
		_ = runtime.Close()
	}
}

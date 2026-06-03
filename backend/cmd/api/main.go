package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/handlers"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"github.com/kurodakayn/mpp-backend/internal/redisclient"
	"github.com/kurodakayn/mpp-backend/internal/services"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	"github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	)

const (
	jwtSecretEnv       = "JWT_SECRET"
	appEnvEnv          = "APP_ENV"
	mockLoginFlagEnv   = "ENABLE_MOCK_LOGIN"
	nodeEnvFallbackEnv = "NODE_ENV"
	shutdownTimeout    = 15 * time.Second
)

func main() {
	rootCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	// Load .env file if it exists
	_ = godotenv.Load()

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
		dashboardService.StartPublishWorker(rootCtx)
	}

	adminDashboardHandler := handlers.NewDashboardHandler(dashboardService)
	userDashboardHandler := handlers.NewUserDashboardHandler(dashboardService)
	userDashboardHandler.UseAIContentEditor(services.NewAIServiceClientFromEnv())
	mockLogin := mockLoginEnabled()
	authHandler := handlers.NewAuthHandler(db.DB, jwtSigningKey)
	authHandler.SetUsernameLoginEnabled(mockLogin)

	if redisClient != nil {
		browserSessionService.UseRedis(redisClient)
		browserSessionService.StartCleanupWorker(rootCtx)
	}
	browserSessionHandler := handlers.NewBrowserSessionHandler(browserSessionService)

	e := echo.New()
	ready := atomic.Bool{}
	ready.Store(true)

	// Middleware
	e.Use(echoMiddleware.Logger())
	e.Use(echoMiddleware.Recover())

	// Public Routes
	registerHealthRoutes(e, &ready, db.DB, redisClient)
	e.GET("/ping", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"message": "pong",
		})
	})
	if mockLogin {
		e.POST("/api/auth/mock-login", authHandler.MockLogin)
	}
	e.POST("/api/auth/login", authHandler.Login)
	e.GET("/api/user/dashboard/settings/x/oauth2/callback", userDashboardHandler.CompleteXOAuth2)

	// Admin APIs (In a real app, protect this with an Admin Auth middleware)
	adminGroup := e.Group("/api/admin/dashboard")
	adminGroup.GET("/stats", adminDashboardHandler.GetStats)
	adminGroup.GET("/projects", adminDashboardHandler.ListProjects)
	adminGroup.GET("/projects/:id/publications", adminDashboardHandler.GetProjectPublications)

	// User / Personal Center APIs (Protected by JWT)
	userGroup := e.Group("/api/user/dashboard")
	userGroup.Use(echojwt.WithConfig(middleware.GetJWTConfig(jwtSigningKey)))
	rateLimitConfig, err := middleware.RateLimitConfigFromEnv(redisClient)
	if err != nil {
		log.Fatal(err)
	}
	if rateLimitConfig.Enabled {
		userGroup.Use(middleware.ApplicationRateLimiter(rateLimitConfig))
	}

	userGroup.GET("/stats", userDashboardHandler.GetMyStats)
	userGroup.GET("/projects", userDashboardHandler.ListMyProjects)
	userGroup.POST("/projects", userDashboardHandler.CreateProject)
	userGroup.GET("/projects/:id", userDashboardHandler.GetMyProject)
	userGroup.PUT("/projects/:id", userDashboardHandler.UpdateProject)
	userGroup.PATCH("/projects/:id/content", userDashboardHandler.SaveProjectContent)
	userGroup.PATCH("/projects/:id/platforms", userDashboardHandler.SaveProjectPlatforms)
	userGroup.GET("/projects/:id/publications", userDashboardHandler.GetMyProjectPublications)
	userGroup.POST("/projects/:id/prepublish/sync", userDashboardHandler.SyncProjectPrepublish)
	userGroup.PUT("/projects/:id/prepublish/:platform", userDashboardHandler.UpdateProjectPrepublishDraft)
	userGroup.POST("/projects/:id/publish", userDashboardHandler.PublishProject)
	userGroup.POST("/projects/:id/publish-sessions/douyin", userDashboardHandler.StartDouyinPublishSession)
	userGroup.POST("/ai/content/edit", userDashboardHandler.EditContentWithAI)
	userGroup.POST("/ai/content/edit/stream", userDashboardHandler.StreamEditContentWithAI)
	userGroup.POST("/ai/prepublish/edit", userDashboardHandler.EditPrepublishWithAI)
	userGroup.POST("/ai/prepublish/edit/stream", userDashboardHandler.StreamEditPrepublishWithAI)
	userGroup.GET("/settings/wechat/account", userDashboardHandler.GetWechatAccount)
	userGroup.PUT("/settings/wechat/account", userDashboardHandler.SaveWechatAccount)
	userGroup.POST("/settings/wechat/test", userDashboardHandler.TestWechatAccount)
	userGroup.GET("/settings/douyin/account", userDashboardHandler.GetDouyinAccount)
	userGroup.GET("/settings/zhihu/account", userDashboardHandler.GetZhihuAccount)
	userGroup.GET("/settings/x/account", userDashboardHandler.GetXAccount)
	userGroup.PUT("/settings/x/account", userDashboardHandler.SaveXAccount)
	userGroup.POST("/settings/x/test", userDashboardHandler.TestXAccount)
	userGroup.GET("/settings/x/oauth2/start", userDashboardHandler.StartXOAuth2)

	// Remote Browser Session Routes
	userGroup.POST("/settings/platforms/:platform/browser-session", browserSessionHandler.StartSession)
	userGroup.GET("/browser-sessions/:id", browserSessionHandler.GetSession)
	userGroup.GET("/browser-sessions/:id/stream", browserSessionHandler.StreamSession)
	userGroup.GET("/browser-sessions/:id/stream/*", browserSessionHandler.StreamSession)
	userGroup.POST("/browser-sessions/:id/complete", browserSessionHandler.CompleteSession)
	userGroup.DELETE("/browser-sessions/:id", browserSessionHandler.CancelSession)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- e.Start(":" + port)
	}()

	select {
	case err := <-serverErrors:
		if err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal(err)
		}
	case <-rootCtx.Done():
		ready.Store(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := e.Shutdown(shutdownCtx); err != nil {
			e.Logger.Fatal(err)
		}
		if redisClient != nil {
			_ = redisClient.Close()
		}
	}
}

func registerHealthRoutes(e *echo.Echo, ready *atomic.Bool, sqlDB *gorm.DB, redisClient *redis.Client) {
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "healthy"})
	})
	e.GET("/ready", func(c echo.Context) error {
		if !ready.Load() {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		}

		ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
		defer cancel()

		if sqlDB != nil {
			dbObj, err := sqlDB.DB()
			if err != nil {
				return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "dependency": "database"})
			}
			if err := dbObj.PingContext(ctx); err != nil {
				return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "dependency": "database"})
			}
		}

		if redisClient != nil {
			if err := redisClient.Ping(ctx).Err(); err != nil {
				return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "dependency": "redis"})
			}
		}

		return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
	})
}

func requiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s must be set", name)
	}
	return value, nil
}

func mockLoginEnabled() bool {
	localEnv := isLocalEnvironment(os.Getenv(appEnvEnv)) || isLocalEnvironment(os.Getenv(nodeEnvFallbackEnv))
	return envFlagEnabled(mockLoginFlagEnv) && localEnv
}

func envFlagEnabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func isLocalEnvironment(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "local", "dev", "development":
		return true
	default:
		return false
	}
}

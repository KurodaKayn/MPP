package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/handlers"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"github.com/kurodakayn/mpp-backend/internal/redisclient"
	"github.com/kurodakayn/mpp-backend/internal/services"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
)

const (
	jwtSecretEnv       = "JWT_SECRET"
	appEnvEnv          = "APP_ENV"
	mockLoginFlagEnv   = "ENABLE_MOCK_LOGIN"
	nodeEnvFallbackEnv = "NODE_ENV"
)

func main() {
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
	if redisClient != nil {
		defer redisClient.Close()
		dashboardService.UseRedis(redisClient)
		dashboardService.StartPublishWorker(context.Background())
	}
	adminDashboardHandler := handlers.NewDashboardHandler(dashboardService)
	userDashboardHandler := handlers.NewUserDashboardHandler(dashboardService)
	userDashboardHandler.UseAIContentEditor(services.NewAIServiceClientFromEnv())
	mockLogin := mockLoginEnabled()
	authHandler := handlers.NewAuthHandler(db.DB, jwtSigningKey)
	authHandler.SetUsernameLoginEnabled(mockLogin)

	// Remote Browser Session (New)
	var workerClient publisher.BrowserWorkerClient
	workerURL := os.Getenv("BROWSER_WORKER_URL")
	if workerURL != "" {
		workerClient = publisher.NewHttpBrowserWorkerClient(workerURL)
	} else {
		workerClient = publisher.NewMockBrowserWorkerClient()
	}

	cookieStore := publisher.NewCookieStore(db.DB)
	browserSessionService := browsersession.NewBrowserSessionService(db.DB, workerClient, cookieStore)
	if redisClient != nil {
		browserSessionService.UseRedis(redisClient)
		browserSessionService.StartCleanupWorker(context.Background())
	}
	browserSessionHandler := handlers.NewBrowserSessionHandler(browserSessionService)

	e := echo.New()

	// Middleware
	e.Use(echoMiddleware.Logger())
	e.Use(echoMiddleware.Recover())

	// Remote Browser Stream (Protected by one-time token, not JWT)
	e.GET("/api/browser-stream/:id", browserSessionHandler.StreamSession)
	e.GET("/api/browser-stream/:id/*", browserSessionHandler.StreamSession)

	// Public Routes
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
	userGroup.POST("/browser-sessions/:id/complete", browserSessionHandler.CompleteSession)
	userGroup.DELETE("/browser-sessions/:id", browserSessionHandler.CancelSession)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start server
	e.Logger.Fatal(e.Start(":" + port))
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

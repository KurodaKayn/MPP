package main

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	dbobs "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/handlers"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/observability"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type serverConfig struct {
	runtimeConfig backendRuntimeConfig
	jwtSigningKey []byte
	redisClient   *redis.Client
	mockLogin     bool
	ready         *atomic.Bool
	sqlDB         *gorm.DB
}

type serverHandlers struct {
	adminDashboard *handlers.DashboardHandler
	userDashboard  *handlers.UserDashboardHandler
	auth           *handlers.AuthHandler
	browserSession *handlers.BrowserSessionHandler
	collabDocument *handlers.CollabDocumentHandler
}

func newServer(config serverConfig, h serverHandlers) (*echo.Echo, error) {
	e := echo.New()
	observabilitySuite := observability.New(config.runtimeConfig.serviceName())
	observabilitySuite.RegisterRoutes(e)
	if err := dbobs.InstallQueryObserver(config.sqlDB, observabilitySuite.DatabaseQueryObserver()); err != nil {
		return nil, err
	}

	e.Use(observabilitySuite.Middleware())
	e.Use(echoMiddleware.Recover())
	registerExtensionCORS(e, config.runtimeConfig.extensionAllowedOrigins)
	registerPublicRoutes(e, config)

	if config.runtimeConfig.servesAPI() {
		if err := registerAPIRoutes(e, config, h); err != nil {
			return nil, err
		}
	}

	return e, nil
}

func registerExtensionCORS(e *echo.Echo, allowedOrigins []string) {
	if len(allowedOrigins) == 0 {
		return
	}

	e.Use(echoMiddleware.CORSWithConfig(echoMiddleware.CORSConfig{
		AllowOrigins: allowedOrigins,
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		// Extension workbench requests authenticate with Authorization: Bearer <web JWT>.
		// Existing SameSite=Lax web cookies are not the direct extension-origin auth path.
		AllowHeaders:     []string{echo.HeaderAuthorization, echo.HeaderContentType},
		AllowCredentials: true,
	}))
}

func registerPublicRoutes(e *echo.Echo, config serverConfig) {
	e.GET("/ping", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"message": "pong",
		})
	})
	registerHealthRoutes(e, config.ready, config.sqlDB, config.redisClient)
}

func registerHealthRoutes(e *echo.Echo, ready *atomic.Bool, sqlDB *gorm.DB, redisClient *redis.Client) {
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "healthy"})
	})
	e.GET("/ready", func(c echo.Context) error {
		if ready != nil && !ready.Load() {
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

func registerAPIRoutes(e *echo.Echo, config serverConfig, h serverHandlers) error {
	registerAuthRoutes(e, config, h)
	registerAdminDashboardRoutes(e, h)
	e.POST("/api/user/dashboard/extension/events", h.userDashboard.RecordExtensionEvent)
	if err := registerUserDashboardRoutes(e, config, h); err != nil {
		return err
	}
	return registerCollabRoutes(e, config, h)
}

func registerAuthRoutes(e *echo.Echo, config serverConfig, h serverHandlers) {
	if config.mockLogin {
		e.POST("/api/auth/mock-login", h.auth.MockLogin)
	}
	e.POST("/api/auth/login", h.auth.Login)
	e.POST("/api/auth/register", h.auth.Register)
	e.POST("/api/auth/send-code", h.auth.SendCode)
	e.POST("/api/auth/reset-password", h.auth.ResetPassword)
	e.GET("/api/user/dashboard/settings/x/oauth2/callback", h.userDashboard.CompleteXOAuth2)
}

func registerAdminDashboardRoutes(e *echo.Echo, h serverHandlers) {
	adminGroup := e.Group("/api/admin/dashboard")
	adminGroup.GET("/stats", h.adminDashboard.GetStats)
	adminGroup.GET("/projects", h.adminDashboard.ListProjects)
	adminGroup.GET("/projects/:id/publications", h.adminDashboard.GetProjectPublications)
}

func registerUserDashboardRoutes(e *echo.Echo, config serverConfig, h serverHandlers) error {
	userGroup := e.Group("/api/user/dashboard")
	userGroup.Use(echojwt.WithConfig(middleware.GetJWTConfig(config.jwtSigningKey)))
	rateLimitConfig, err := middleware.RateLimitConfigFromEnv(config.redisClient)
	if err != nil {
		return err
	}
	if rateLimitConfig.Enabled {
		userGroup.Use(middleware.ApplicationRateLimiter(rateLimitConfig))
	}

	userGroup.GET("/stats", h.userDashboard.GetMyStats)
	userGroup.GET("/extension/session", h.userDashboard.GetExtensionSession)
	userGroup.GET("/extension/prepublish", h.userDashboard.ListExtensionPrepublish)
	userGroup.POST("/extension/handoffs", h.userDashboard.CreateExtensionHandoff)
	userGroup.GET("/projects", h.userDashboard.ListMyProjects)
	userGroup.POST("/projects", h.userDashboard.CreateProject)
	userGroup.GET("/projects/:id", h.userDashboard.GetMyProject)
	userGroup.PUT("/projects/:id", h.userDashboard.UpdateProject)
	userGroup.PATCH("/projects/:id/content", h.userDashboard.SaveProjectContent)
	userGroup.PATCH("/projects/:id/platforms", h.userDashboard.SaveProjectPlatforms)
	userGroup.GET("/projects/:id/publications", h.userDashboard.GetMyProjectPublications)
	userGroup.POST("/projects/:id/prepublish/sync", h.userDashboard.SyncProjectPrepublish)
	userGroup.PUT("/projects/:id/prepublish/:platform", h.userDashboard.UpdateProjectPrepublishDraft)
	userGroup.POST("/projects/:id/publish", h.userDashboard.PublishProject)
	userGroup.POST("/projects/:id/publish-sessions/douyin", h.userDashboard.StartDouyinPublishSession)
	userGroup.POST("/ai/content/edit", h.userDashboard.EditContentWithAI)
	userGroup.POST("/ai/content/edit/stream", h.userDashboard.StreamEditContentWithAI)
	userGroup.POST("/ai/prepublish/edit", h.userDashboard.EditPrepublishWithAI)
	userGroup.POST("/ai/prepublish/edit/stream", h.userDashboard.StreamEditPrepublishWithAI)
	userGroup.GET("/settings/wechat/account", h.userDashboard.GetWechatAccount)
	userGroup.PUT("/settings/wechat/account", h.userDashboard.SaveWechatAccount)
	userGroup.POST("/settings/wechat/test", h.userDashboard.TestWechatAccount)
	userGroup.GET("/settings/douyin/account", h.userDashboard.GetDouyinAccount)
	userGroup.GET("/settings/zhihu/account", h.userDashboard.GetZhihuAccount)
	userGroup.GET("/settings/x/account", h.userDashboard.GetXAccount)
	userGroup.PUT("/settings/x/account", h.userDashboard.SaveXAccount)
	userGroup.POST("/settings/x/test", h.userDashboard.TestXAccount)
	userGroup.GET("/settings/x/oauth2/start", h.userDashboard.StartXOAuth2)

	userGroup.POST("/settings/platforms/:platform/browser-session", h.browserSession.StartSession)
	userGroup.GET("/browser-sessions/:id", h.browserSession.GetSession)
	userGroup.GET("/browser-sessions/:id/stream", h.browserSession.StreamSession)
	userGroup.GET("/browser-sessions/:id/stream/*", h.browserSession.StreamSession)
	userGroup.POST("/browser-sessions/:id/complete", h.browserSession.CompleteSession)
	userGroup.DELETE("/browser-sessions/:id", h.browserSession.CancelSession)
	return nil
}

func registerCollabRoutes(e *echo.Echo, config serverConfig, h serverHandlers) error {
	collabGroup := e.Group("/api/collab")
	collabGroup.Use(echojwt.WithConfig(middleware.GetJWTConfig(config.jwtSigningKey)))
	rateLimitConfig, err := middleware.RateLimitConfigFromEnv(config.redisClient)
	if err != nil {
		return err
	}
	if rateLimitConfig.Enabled {
		collabGroup.Use(middleware.ApplicationRateLimiter(rateLimitConfig))
	}

	collabGroup.GET("/documents", h.collabDocument.ListDocuments)
	collabGroup.POST("/documents", h.collabDocument.CreateDocument)
	return nil
}

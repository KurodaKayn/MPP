package main

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/app"
	dbobs "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/handlers"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/observability"
)

type serverConfig struct {
	runtimeConfig      app.RuntimeConfig
	jwtSigningKey      []byte
	redisClient        *redis.Client
	mockLogin          bool
	ready              *atomic.Bool
	sqlDB              *gorm.DB
	dbRouter           *dbobs.Router
	observabilitySuite *observability.Suite
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
	observabilitySuite := config.observabilitySuite
	if observabilitySuite == nil {
		observabilitySuite = observability.New(config.runtimeConfig.ServiceName())
	}
	observabilitySuite.RegisterRoutes(e)
	if config.dbRouter != nil {
		if err := config.dbRouter.InstallQueryObserver(observabilitySuite.DatabaseQueryObserver()); err != nil {
			return nil, err
		}
	} else {
		if err := dbobs.InstallQueryObserver(config.sqlDB, observabilitySuite.DatabaseQueryObserver()); err != nil {
			return nil, err
		}
	}

	e.Use(observabilitySuite.Middleware())
	e.Use(echoMiddleware.Recover())
	e.Use(middleware.StickyWriterWithConfig(middleware.StickyWriterConfig{
		Secret: config.jwtSigningKey,
		Secure: app.SecureCookiesByDefault(),
	}))
	registerExtensionCORS(e, config.runtimeConfig.ExtensionAllowedOrigins)
	registerPublicRoutes(e, config)

	if config.runtimeConfig.ServesAPI() {
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
	app.RegisterHealthRoutes(e, config.ready, config.sqlDB, config.redisClient)
}

func registerAPIRoutes(e *echo.Echo, config serverConfig, h serverHandlers) error {
	registerAuthRoutes(e, config, h)
	registerAdminDashboardRoutes(e, config, h)
	e.POST("/api/user/dashboard/extension/events", h.userDashboard.RecordExtensionEvent)
	registerInternalRoutes(e, h)
	if err := registerUserDashboardRoutes(e, config, h); err != nil {
		return err
	}
	if err := registerWorkspaceRoutes(e, config, h); err != nil {
		return err
	}
	return registerCollabRoutes(e, config, h)
}

func registerInternalRoutes(e *echo.Echo, h serverHandlers) {
	e.POST("/internal/media/resolve", h.userDashboard.ResolveMediaObjectRef, requireInternalToken)
}

func requireInternalToken(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		expected := strings.TrimSpace(os.Getenv("CONTENT_PIPELINE_INTERNAL_TOKEN"))
		if expected == "" {
			return c.JSON(http.StatusServiceUnavailable, map[string]any{
				"error": map[string]string{
					"code":    "internal_token_unconfigured",
					"message": "internal token is not configured",
				},
			})
		}

		token := strings.TrimSpace(c.Request().Header.Get("X-MPP-Internal-Token"))
		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
			return c.JSON(http.StatusUnauthorized, map[string]any{
				"error": map[string]string{
					"code":    "unauthorized",
					"message": "invalid internal token",
				},
			})
		}

		return next(c)
	}
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

func registerAdminDashboardRoutes(e *echo.Echo, config serverConfig, h serverHandlers) {
	adminGroup := e.Group("/api/admin/dashboard")
	adminGroup.Use(echojwt.WithConfig(middleware.GetJWTConfig(config.jwtSigningKey)))
	adminGroup.Use(middleware.RequireAdmin())
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
	userGroup.GET("/projects/:id/collaborators", h.userDashboard.ListProjectCollaborators)
	userGroup.POST("/projects/:id/collaborators", h.userDashboard.AddProjectCollaborator)
	userGroup.PATCH("/projects/:id/collaborators/:userId", h.userDashboard.UpdateProjectCollaborator)
	userGroup.DELETE("/projects/:id/collaborators/:userId", h.userDashboard.RemoveProjectCollaborator)
	userGroup.GET("/projects/:id/activity", h.userDashboard.ListProjectActivities)
	userGroup.GET("/projects/:id/comments", h.userDashboard.ListProjectComments)
	userGroup.POST("/projects/:id/comments", h.userDashboard.CreateProjectComment)
	userGroup.PATCH("/projects/:id/comments/:commentId", h.userDashboard.UpdateProjectComment)
	userGroup.GET("/projects/:id/versions", h.userDashboard.ListProjectVersions)
	userGroup.POST("/projects/:id/versions/:versionId/restore", h.userDashboard.RestoreProjectVersion)
	userGroup.GET("/projects/:id/share-links", h.userDashboard.ListProjectShareLinks)
	userGroup.POST("/projects/:id/share-links", h.userDashboard.CreateProjectShareLink)
	userGroup.DELETE("/projects/:id/share-links/:linkId", h.userDashboard.RevokeProjectShareLink)
	userGroup.POST("/project-share-links/:token/accept", h.userDashboard.AcceptProjectShareLink)
	userGroup.POST("/projects/:id/collab/session", h.userDashboard.CreateProjectCollabSession)
	userGroup.PATCH("/projects/:id/content", h.userDashboard.SaveProjectContent)
	userGroup.PATCH("/projects/:id/platforms", h.userDashboard.SaveProjectPlatforms)
	userGroup.POST("/projects/:id/media/uploads", h.userDashboard.CreateProjectMediaUpload)
	userGroup.POST("/media/:id/complete", h.userDashboard.CompleteMediaUpload)
	userGroup.POST("/media/resolve", h.userDashboard.ResolveMediaAssets)
	userGroup.DELETE("/media/:id", h.userDashboard.DeleteMediaAsset)
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

func registerWorkspaceRoutes(e *echo.Echo, config serverConfig, h serverHandlers) error {
	workspaceGroup := e.Group("/api/workspaces")
	workspaceGroup.Use(echojwt.WithConfig(middleware.GetJWTConfig(config.jwtSigningKey)))
	rateLimitConfig, err := middleware.RateLimitConfigFromEnv(config.redisClient)
	if err != nil {
		return err
	}
	if rateLimitConfig.Enabled {
		workspaceGroup.Use(middleware.ApplicationRateLimiter(rateLimitConfig))
	}

	workspaceGroup.GET("", h.userDashboard.ListWorkspaces)
	workspaceGroup.POST("", h.userDashboard.CreateWorkspace)
	workspaceGroup.GET("/:id/projects", h.userDashboard.ListWorkspaceProjects)
	workspaceGroup.POST("/:id/projects", h.userDashboard.CreateWorkspaceProject)
	workspaceGroup.GET("/:id/activity", h.userDashboard.ListWorkspaceActivities)
	workspaceGroup.GET("/:id/members", h.userDashboard.ListWorkspaceMembers)
	workspaceGroup.GET("/:id/invites", h.userDashboard.ListWorkspaceInvites)
	workspaceGroup.GET("/:id", h.userDashboard.GetWorkspace)
	workspaceGroup.PATCH("/:id", h.userDashboard.UpdateWorkspace)
	workspaceGroup.POST("/:id/members", h.userDashboard.AddWorkspaceMember)
	workspaceGroup.POST("/:id/invites", h.userDashboard.CreateWorkspaceInvite)
	workspaceGroup.POST("/invites/accept", h.userDashboard.AcceptWorkspaceInvite)
	workspaceGroup.PATCH("/:id/members/:userId", h.userDashboard.UpdateWorkspaceMember)
	workspaceGroup.DELETE("/:id/members/:userId", h.userDashboard.RemoveWorkspaceMember)
	workspaceGroup.DELETE("/:id/invites/:inviteId", h.userDashboard.RevokeWorkspaceInvite)
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
	collabGroup.GET("/documents/:id", h.collabDocument.GetDocument)
	collabGroup.POST("/documents", h.collabDocument.CreateDocument)
	collabGroup.PATCH("/documents/:id", h.collabDocument.UpdateDocument)
	collabGroup.POST("/documents/:id/session", h.collabDocument.CreateSession)
	return nil
}

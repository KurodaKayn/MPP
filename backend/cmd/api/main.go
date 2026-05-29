package main

import (
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/db"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/models"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/services"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/handlers"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/middleware"
	"net/http"
	"os"
)

func main() {
	// Load .env file if it exists
	_ = godotenv.Load()

	// Initialize Database
	db.InitDB()

	// Auto Migrate
	db.DB.AutoMigrate(
		&models.User{},
		&models.Project{},
		&models.ProjectPlatformPublication{},
	)

	// Initialize Services and Handlers
	dashboardService := services.NewDashboardService(db.DB)
	adminDashboardHandler := handlers.NewDashboardHandler(dashboardService)
	userDashboardHandler := handlers.NewUserDashboardHandler(dashboardService)
	authHandler := handlers.NewAuthHandler(db.DB)

	e := echo.New()

	// Middleware
	e.Use(echoMiddleware.Logger())
	e.Use(echoMiddleware.Recover())

	// Public Routes
	e.GET("/ping", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"message": "pong",
		})
	})
	
	e.POST("/api/auth/mock-login", authHandler.MockLogin)

	// Admin APIs (In a real app, protect this with an Admin Auth middleware)
	adminGroup := e.Group("/api/admin/dashboard")
	adminGroup.GET("/stats", adminDashboardHandler.GetStats)
	adminGroup.GET("/projects", adminDashboardHandler.ListProjects)
	adminGroup.GET("/projects/:id/publications", adminDashboardHandler.GetProjectPublications)

	// User / Personal Center APIs (Protected by JWT)
	userGroup := e.Group("/api/user/dashboard")
	userGroup.Use(echojwt.WithConfig(middleware.GetJWTConfig()))
	userGroup.GET("/stats", userDashboardHandler.GetMyStats)
	userGroup.GET("/projects", userDashboardHandler.ListMyProjects)
	userGroup.GET("/projects/:id/publications", userDashboardHandler.GetMyProjectPublications)

	// AI Proxy example
	e.POST("/api/ai/calibrate", func(c echo.Context) error {
		// In a real app, this would proxy to the AI service
		return c.JSON(http.StatusOK, map[string]string{
			"status": "pending",
			"message": "AI calibration endpoint initialized",
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start server
	e.Logger.Fatal(e.Start(":" + port))
}

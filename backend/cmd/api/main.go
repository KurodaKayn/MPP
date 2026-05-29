package main

import (
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/db"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/models"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/services"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/handlers"
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
	dashboardHandler := handlers.NewDashboardHandler(dashboardService)

	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Routes
	e.GET("/ping", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"message": "pong",
		})
	})

	// Dashboard APIs
	e.GET("/api/dashboard/stats", dashboardHandler.GetStats)
	e.GET("/api/projects", dashboardHandler.ListProjects)
	e.GET("/api/projects/:id/publications", dashboardHandler.GetProjectPublications)

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

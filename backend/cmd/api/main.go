package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/db"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/handlers"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/middleware"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/services"
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
	adminDashboardHandler := handlers.NewDashboardHandler(dashboardService)
	userDashboardHandler := handlers.NewUserDashboardHandler(dashboardService)
	authHandler := handlers.NewAuthHandler(db.DB, jwtSigningKey)

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
	if mockLoginEnabled() {
		e.POST("/api/auth/mock-login", authHandler.MockLogin)
	}

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
	userGroup.GET("/projects/:id/publications", userDashboardHandler.GetMyProjectPublications)
	userGroup.POST("/projects/:id/publish", userDashboardHandler.PublishProject)

	// AI Proxy example
	e.POST("/api/ai/calibrate", func(c echo.Context) error {
		// In a real app, this would proxy to the AI service
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "pending",
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

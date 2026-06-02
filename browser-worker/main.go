package main

import (
	"context"
	"log"

	browsercontainer "github.com/kurodakayn/mpp-browser-worker/internal/container"
	"github.com/kurodakayn/mpp-browser-worker/internal/observability"
	"github.com/kurodakayn/mpp-browser-worker/internal/server"
	"github.com/kurodakayn/mpp-browser-worker/internal/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	e := echo.New()
	observabilitySuite := observability.New("browser-worker")
	observabilitySuite.RegisterRoutes(e)
	e.Use(observabilitySuite.Middleware())
	e.Use(middleware.Recover())

	containers, err := browsercontainer.NewManager()
	if err != nil {
		log.Fatalf("Failed to initialize Docker manager: %v", err)
	}

	sessions := session.NewManager()
	stateStore, err := session.NewRedisStateStoreFromEnv(context.Background())
	if err != nil {
		log.Fatalf("Failed to initialize Redis state store: %v", err)
	}
	defer stateStore.Close()

	app := server.New(containers, sessions, stateStore)
	app.RegisterRoutes(e)

	e.Logger.Fatal(e.Start(":8081"))
}

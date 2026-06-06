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

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	browsercontainer "github.com/kurodakayn/mpp-browser-worker/internal/container"
	"github.com/kurodakayn/mpp-browser-worker/internal/observability"
	"github.com/kurodakayn/mpp-browser-worker/internal/server"
	"github.com/kurodakayn/mpp-browser-worker/internal/session"
)

const shutdownTimeout = 15 * time.Second

func main() {
	rootCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

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

	app := server.New(containers, sessions, stateStore)
	ready := atomic.Bool{}
	ready.Store(true)
	registerHealthRoutes(e, &ready, stateStore)
	app.RegisterRoutes(e)

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- e.Start(":8081")
	}()

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			e.Logger.Fatal(err)
		}
	case <-rootCtx.Done():
		ready.Store(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := e.Shutdown(shutdownCtx); err != nil {
			e.Logger.Fatal(err)
		}
		app.ShutdownSessions(shutdownCtx)
		if err := stateStore.Close(); err != nil {
			log.Printf("Failed to close Redis state store: %v", err)
		}
	}
}

func registerHealthRoutes(e *echo.Echo, ready *atomic.Bool, stateStore *session.RedisStateStore) {
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "healthy"})
	})
	e.GET("/ready", func(c echo.Context) error {
		if !ready.Load() {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		}
		ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
		defer cancel()
		if err := stateStore.Ping(ctx); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "dependency": "redis"})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
	})
}

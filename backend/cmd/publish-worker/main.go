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
)

const shutdownTimeout = 15 * time.Second

func main() {
	rootCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	_ = godotenv.Load()

	db.InitDB()

	runtime, err := app.NewRuntime(rootCtx, app.RuntimeWiringConfig{
		Mode: app.RuntimeModePublishWorker,
		RuntimeConfig: app.RuntimeConfig{
			ProcessRole:  app.ProcessRoleWorker,
			RequireRedis: true,
		},
		SQLDB:    db.DB,
		DBRouter: db.DefaultRouter,
	})
	if err != nil {
		log.Fatal(err)
	}

	ready := atomic.Bool{}
	ready.Store(true)
	server, err := app.NewHealthServer(app.HealthServerConfig{
		Ready:              &ready,
		RedisClient:        runtime.RedisClient,
		ObservabilitySuite: runtime.ObservabilitySuite,
		DBRouter:           db.DefaultRouter,
		SQLDB:              db.DB,
		ServiceName:        app.PublishWorkerServiceName,
	})
	if err != nil {
		log.Fatal(err)
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Start(":" + app.PortFromEnv())
	}()

	workerErrors := runtime.StartPublishWorkerMode(rootCtx)

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	case err := <-workerErrors.Email:
		ready.Store(false)
		log.Fatalf("email worker stopped: %v", err)
	case err := <-workerErrors.Publish:
		ready.Store(false)
		if err != nil {
			log.Fatalf("publish worker stopped: %v", err)
		}
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

package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/redis/go-redis/v9"

	"github.com/kurodakayn/mpp-backend/internal/app"
	"github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/observability"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"github.com/kurodakayn/mpp-backend/internal/redisclient"
	"github.com/kurodakayn/mpp-backend/internal/services"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	"github.com/kurodakayn/mpp-backend/internal/services/email"
)

const shutdownTimeout = 15 * time.Second

func main() {
	rootCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	_ = godotenv.Load()

	db.InitDB()

	redisClient, err := redisclient.NewFromEnv(context.Background())
	if errors.Is(err, redisclient.ErrNotConfigured) {
		log.Fatal("REDIS_ADDR must be set for publish-worker")
	} else if err != nil {
		log.Fatal(err)
	}

	workerClient := app.NewBrowserWorkerClientFromEnv()
	browserSessionService := browsersession.NewBrowserSessionService(db.DB, workerClient, publisher.NewCookieStore(db.DB))
	browserSessionService.UseRedis(redisClient)

	observabilitySuite := observability.New(app.PublishWorkerServiceName)
	dashboardService := services.NewDashboardServiceWithRouter(db.DB, db.DefaultRouter)
	dashboardService.SetPublishJobObserver(observabilitySuite.PublishJobObserver())
	dashboardService.SetBrowserWorkerClient(workerClient)
	dashboardService.SetBrowserSessionService(browserSessionService)
	dashboardService.UseRedis(redisClient)

	baseEmailService, err := app.NewBaseEmailServiceFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	asyncEmailService := email.NewAsyncEmailService(redisClient)

	ready := atomic.Bool{}
	ready.Store(true)
	server, err := newHealthServer(&ready, redisClient, observabilitySuite, db.DefaultRouter)
	if err != nil {
		log.Fatal(err)
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Start(":" + portFromEnv())
	}()

	workerErrors := make(chan error, 1)
	var workerWG sync.WaitGroup
	publishWorkerErrors := dashboardService.StartPublishWorkerWithErrors(rootCtx)
	browserSessionService.StartCleanupWorker(rootCtx)
	workerWG.Go(func() {
		if err := asyncEmailService.StartWorker(rootCtx, baseEmailService); err != nil && rootCtx.Err() == nil {
			select {
			case workerErrors <- err:
			default:
				log.Printf("email worker stopped with error: %v", err)
			}
		}
	})

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	case err := <-workerErrors:
		ready.Store(false)
		log.Fatalf("email worker stopped: %v", err)
	case err := <-publishWorkerErrors:
		ready.Store(false)
		if err != nil {
			log.Fatalf("publish worker stopped: %v", err)
		}
	case <-rootCtx.Done():
		ready.Store(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Fatal(err)
		}
		workerWG.Wait()
		if redisClient != nil {
			_ = redisClient.Close()
		}
	}
}

func newHealthServer(ready *atomic.Bool, redisClient *redis.Client, observabilitySuite *observability.Suite, router *db.Router) (*echo.Echo, error) {
	e := echo.New()
	if observabilitySuite == nil {
		observabilitySuite = observability.New(app.PublishWorkerServiceName)
	}
	observabilitySuite.RegisterRoutes(e)
	if router != nil {
		if err := router.InstallQueryObserver(observabilitySuite.DatabaseQueryObserver()); err != nil {
			return nil, err
		}
	} else {
		if err := db.InstallQueryObserver(db.DB, observabilitySuite.DatabaseQueryObserver()); err != nil {
			return nil, err
		}
	}
	e.Use(observabilitySuite.Middleware())
	e.Use(echoMiddleware.Recover())
	app.RegisterHealthRoutes(e, ready, db.DB, redisClient)
	return e, nil
}

func portFromEnv() string {
	port := os.Getenv("PORT")
	if port == "" {
		return "8080"
	}
	return port
}

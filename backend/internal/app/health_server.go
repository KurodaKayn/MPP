package app

import (
	"os"
	"sync/atomic"

	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/observability"
)

const defaultHTTPPort = "8080"

type HealthServerConfig struct {
	Ready              *atomic.Bool
	RedisClient        *redis.Client
	ObservabilitySuite *observability.Suite
	DBRouter           *db.Router
	SQLDB              *gorm.DB
	ServiceName        string
}

func PortFromEnv() string {
	port := os.Getenv("PORT")
	if port == "" {
		return defaultHTTPPort
	}
	return port
}

func NewHealthServer(config HealthServerConfig) (*echo.Echo, error) {
	e := echo.New()
	observabilitySuite := config.ObservabilitySuite
	if observabilitySuite == nil {
		serviceName := config.ServiceName
		if serviceName == "" {
			serviceName = PublishWorkerServiceName
		}
		observabilitySuite = observability.New(serviceName)
	}
	observabilitySuite.RegisterRoutes(e)
	if config.DBRouter != nil {
		if err := config.DBRouter.InstallQueryObserver(observabilitySuite.DatabaseQueryObserver()); err != nil {
			return nil, err
		}
		config.DBRouter.InstallReplicaLagObserver(observabilitySuite.ReplicaLagObserver())
	} else if config.SQLDB != nil {
		if err := db.InstallQueryObserver(config.SQLDB, observabilitySuite.DatabaseQueryObserver()); err != nil {
			return nil, err
		}
	}
	e.Use(observabilitySuite.Middleware())
	e.Use(echoMiddleware.Recover())
	RegisterHealthRoutes(e, config.Ready, config.SQLDB, config.RedisClient)
	return e, nil
}

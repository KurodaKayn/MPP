package app

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func RegisterHealthRoutes(e *echo.Echo, ready *atomic.Bool, sqlDB *gorm.DB, redisClient *redis.Client) {
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

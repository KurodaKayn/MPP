package redisclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	addrEnv     = "REDIS_ADDR"
	passwordEnv = "REDIS_PASSWORD"
	dbEnv       = "REDIS_DB"
	tlsEnv      = "REDIS_TLS"
)

var ErrNotConfigured = errors.New("redis is not configured")

func NewFromEnv(ctx context.Context) (*redis.Client, error) {
	addr := strings.TrimSpace(os.Getenv(addrEnv))
	if addr == "" {
		return nil, ErrNotConfigured
	}

	db, err := redisDBFromEnv()
	if err != nil {
		return nil, err
	}

	options := &redis.Options{
		Addr:     addr,
		Password: strings.TrimSpace(os.Getenv(passwordEnv)),
		DB:       db,
	}
	if envFlagEnabled(tlsEnv) {
		options.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	client := redis.NewClient(options)
	if err := pingWithRetry(ctx, client); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return client, nil
}

func pingWithRetry(ctx context.Context, client *redis.Client) error {
	var lastErr error
	for range 10 {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		lastErr = client.Ping(pingCtx).Err()
		cancel()
		if lastErr == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return lastErr
}

func redisDBFromEnv() (int, error) {
	raw := strings.TrimSpace(os.Getenv(dbEnv))
	if raw == "" {
		return 0, nil
	}
	db, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid REDIS_DB: %w", err)
	}
	if db < 0 {
		return 0, fmt.Errorf("invalid REDIS_DB: must be non-negative")
	}
	return db, nil
}

func envFlagEnabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

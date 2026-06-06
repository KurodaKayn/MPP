package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"

	"github.com/kurodakayn/mpp-backend/internal/app"
)

func TestHealthRouteReturnsHealthy(t *testing.T) {
	e := echo.New()
	ready := atomic.Bool{}
	ready.Store(true)
	app.RegisterHealthRoutes(e, &ready, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"healthy"}`, rec.Body.String())
}

func TestReadyRouteReturnsReadyWhenDependenciesHealthy(t *testing.T) {
	e := echo.New()
	ready := atomic.Bool{}
	ready.Store(true)

	// Since we mock nil DB and Redis, the handler skips PingContext and assumes healthy
	app.RegisterHealthRoutes(e, &ready, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"ready"}`, rec.Body.String())
}

func TestReadyRouteRejectsWhenDraining(t *testing.T) {
	e := echo.New()
	ready := atomic.Bool{}
	app.RegisterHealthRoutes(e, &ready, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.JSONEq(t, `{"status":"not_ready"}`, rec.Body.String())
}

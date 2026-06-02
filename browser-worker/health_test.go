package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestHealthRouteReturnsHealthy(t *testing.T) {
	e := echo.New()
	ready := atomic.Bool{}
	ready.Store(true)
	registerHealthRoutes(e, &ready, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"healthy"}`, rec.Body.String())
}

func TestReadyRouteRejectsWhenDraining(t *testing.T) {
	e := echo.New()
	ready := atomic.Bool{}
	registerHealthRoutes(e, &ready, nil)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.JSONEq(t, `{"status":"not_ready"}`, rec.Body.String())
}

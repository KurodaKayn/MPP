package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestPortFromEnvDefaultsTo8080(t *testing.T) {
	t.Setenv("PORT", "")

	if got := portFromEnv(); got != "8080" {
		t.Fatalf("expected default port 8080, got %q", got)
	}
}

func TestPortFromEnvUsesConfiguredPort(t *testing.T) {
	t.Setenv("PORT", "9090")

	if got := portFromEnv(); got != "9090" {
		t.Fatalf("expected configured port 9090, got %q", got)
	}
}

func TestHealthServerReadyRouteRejectsWhenDraining(t *testing.T) {
	ready := atomic.Bool{}
	server, err := newHealthServer(&ready, nil)
	if err != nil {
		t.Fatalf("expected health server: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

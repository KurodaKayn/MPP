package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/kurodakayn/mpp-backend/internal/handlers"
)

func TestServerAllowsConfiguredExtensionOriginWithCredentials(t *testing.T) {
	server, err := newServer(serverConfig{
		runtimeConfig: backendRuntimeConfig{
			processRole:             backendProcessRoleAPI,
			extensionAllowedOrigins: []string{"chrome-extension://abc"},
		},
		ready: &atomic.Bool{},
	}, serverHandlers{})
	if err != nil {
		t.Fatalf("expected server: %v", err)
	}

	req := httptest.NewRequest(http.MethodOptions, "/api/user/dashboard/stats", nil)
	req.Header.Set("Origin", "chrome-extension://abc")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected preflight status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "chrome-extension://abc" {
		t.Fatalf("expected configured origin to be allowed, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected credentialed extension requests to be allowed, got %q", got)
	}
}

func TestServerRejectsUnconfiguredExtensionOrigin(t *testing.T) {
	server, err := newServer(serverConfig{
		runtimeConfig: backendRuntimeConfig{
			processRole:             backendProcessRoleAPI,
			extensionAllowedOrigins: []string{"chrome-extension://abc"},
		},
		ready: &atomic.Bool{},
	}, serverHandlers{})
	if err != nil {
		t.Fatalf("expected server: %v", err)
	}

	req := httptest.NewRequest(http.MethodOptions, "/api/user/dashboard/stats", nil)
	req.Header.Set("Origin", "chrome-extension://not-configured")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected unconfigured origin to be rejected, got %q", got)
	}
}

func TestUserDashboardRoutesIncludeExtensionSession(t *testing.T) {
	server, err := newServer(serverConfig{
		runtimeConfig: backendRuntimeConfig{
			processRole: backendProcessRoleAPI,
		},
		jwtSigningKey: []byte("test-secret"),
		ready:         &atomic.Bool{},
	}, serverHandlers{
		userDashboard: &handlers.UserDashboardHandler{},
	})
	if err != nil {
		t.Fatalf("expected server: %v", err)
	}

	for _, route := range server.Routes() {
		if route.Method == http.MethodGet && route.Path == "/api/user/dashboard/extension/session" {
			return
		}
	}

	t.Fatal("expected extension session route to be registered")
}

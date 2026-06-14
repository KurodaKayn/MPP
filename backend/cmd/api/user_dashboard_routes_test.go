package main

import (
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/kurodakayn/mpp-backend/internal/app"
	"github.com/kurodakayn/mpp-backend/internal/handlers"
)

func TestUserDashboardRoutesIncludeProjectDeleteRoute(t *testing.T) {
	server, err := newServer(serverConfig{
		runtimeConfig: app.RuntimeConfig{
			ProcessRole: app.ProcessRoleAPI,
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
		if route.Method == http.MethodDelete && route.Path == "/api/user/dashboard/projects/:id" {
			return
		}
	}

	t.Fatalf("expected user dashboard project delete route to be registered")
}

func TestUserDashboardRoutesIncludeAIGrowthOptimizationRoute(t *testing.T) {
	server, err := newServer(serverConfig{
		runtimeConfig: app.RuntimeConfig{
			ProcessRole: app.ProcessRoleAPI,
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
		if route.Method == http.MethodPost && route.Path == "/api/user/dashboard/projects/:id/ai/optimize" {
			return
		}
	}

	t.Fatalf("expected user dashboard AI growth optimization route to be registered")
}

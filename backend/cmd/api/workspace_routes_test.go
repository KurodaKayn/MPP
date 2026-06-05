package main

import (
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/kurodakayn/mpp-backend/internal/handlers"
)

func TestWorkspaceRoutesIncludeManagementRoutes(t *testing.T) {
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

	expectedRoutes := map[string]bool{
		http.MethodGet + " /api/workspaces":                        false,
		http.MethodPost + " /api/workspaces":                       false,
		http.MethodGet + " /api/workspaces/:id":                    false,
		http.MethodPatch + " /api/workspaces/:id":                  false,
		http.MethodGet + " /api/workspaces/:id/members":            false,
		http.MethodPost + " /api/workspaces/:id/members":           false,
		http.MethodPatch + " /api/workspaces/:id/members/:userId":  false,
		http.MethodDelete + " /api/workspaces/:id/members/:userId": false,
	}
	for _, route := range server.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := expectedRoutes[key]; ok {
			expectedRoutes[key] = true
		}
	}

	for route, registered := range expectedRoutes {
		if !registered {
			t.Fatalf("expected workspace route %s to be registered", route)
		}
	}
}

package main

import (
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/kurodakayn/mpp-backend/internal/handlers"
)

func TestCollabRoutesIncludeDocumentRoutes(t *testing.T) {
	server, err := newServer(serverConfig{
		runtimeConfig: backendRuntimeConfig{
			processRole: backendProcessRoleAPI,
		},
		jwtSigningKey: []byte("test-secret"),
		ready:         &atomic.Bool{},
	}, serverHandlers{
		collabDocument: &handlers.CollabDocumentHandler{},
	})
	if err != nil {
		t.Fatalf("expected server: %v", err)
	}

	expectedRoutes := map[string]bool{
		http.MethodGet + " /api/collab/documents":              false,
		http.MethodGet + " /api/collab/documents/:id":          false,
		http.MethodPatch + " /api/collab/documents/:id":        false,
		http.MethodPost + " /api/collab/documents":             false,
		http.MethodPost + " /api/collab/documents/:id/session": false,
	}
	for _, route := range server.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := expectedRoutes[key]; ok {
			expectedRoutes[key] = true
		}
	}

	for route, registered := range expectedRoutes {
		if !registered {
			t.Fatalf("expected collab route %s to be registered", route)
		}
	}
}

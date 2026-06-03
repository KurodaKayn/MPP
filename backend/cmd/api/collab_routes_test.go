package main

import (
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/kurodakayn/mpp-backend/internal/handlers"
)

func TestCollabRoutesIncludeDocumentCreation(t *testing.T) {
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

	for _, route := range server.Routes() {
		if route.Method == http.MethodPost && route.Path == "/api/collab/documents" {
			return
		}
	}

	t.Fatal("expected collab document creation route to be registered")
}

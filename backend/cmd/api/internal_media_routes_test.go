package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kurodakayn/mpp-backend/internal/app"
	"github.com/kurodakayn/mpp-backend/internal/handlers"
)

func TestInternalMediaResolverRouteRequiresInternalToken(t *testing.T) {
	t.Setenv("CONTENT_PIPELINE_INTERNAL_TOKEN", "test-internal-token")

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

	req := httptest.NewRequest(http.MethodPost, "/internal/media/resolve", strings.NewReader(`{"object_ref":"mpp://media/11111111-1111-4111-8111-111111111111"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing internal token to be rejected with %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

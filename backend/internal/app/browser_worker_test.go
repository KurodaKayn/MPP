package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewBrowserWorkerClientFromEnvSendsInternalToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer worker-token" {
			t.Fatalf("expected internal bearer token header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"worker_session_ref":"ref","status":"ready"}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("BROWSER_WORKER_URL", server.URL)
	t.Setenv(browserWorkerInternalTokenEnv, "worker-token")

	client := NewBrowserWorkerClientFromEnv()
	resp, err := client.GetSession(context.Background(), "ref")
	if err != nil {
		t.Fatalf("expected worker response: %v", err)
	}
	if resp.WorkerSessionRef != "ref" {
		t.Fatalf("expected worker session ref, got %q", resp.WorkerSessionRef)
	}
}

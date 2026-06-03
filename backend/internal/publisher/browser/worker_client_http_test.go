package browser

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHttpBrowserWorkerClientAbsoluteWorkerURL(t *testing.T) {
	client := NewHttpBrowserWorkerClient("http://browser-worker:8081/")

	assert.Equal(t,
		"http://browser-worker:8081/internal/browser-sessions/ref/stream",
		client.absoluteWorkerURL("/internal/browser-sessions/ref/stream"),
	)
	assert.Equal(t,
		"http://stream.example.test/path",
		client.absoluteWorkerURL("http://stream.example.test/path"),
	)
}

func TestHttpBrowserWorkerClientMapsPoolExhaustion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"message":"browser worker session pool exhausted"}`))
	}))
	t.Cleanup(server.Close)
	client := NewHttpBrowserWorkerClient(server.URL)

	_, err := client.CreateSession(context.Background(), StartWorkerSessionRequest{})

	assert.True(t, errors.Is(err, ErrBrowserWorkerPoolExhausted), err)
}

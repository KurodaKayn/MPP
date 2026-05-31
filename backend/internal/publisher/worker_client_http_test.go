package publisher

import (
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

package handlers

import (
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestStreamTokenAndProxyPath(t *testing.T) {
	token, proxyPath := streamTokenAndProxyPath("", "stream-token/vnc.html")
	assert.Equal(t, "stream-token", token)
	assert.Equal(t, "vnc.html", proxyPath)

	token, proxyPath = streamTokenAndProxyPath("query-token", "app/ui.js")
	assert.Equal(t, "query-token", token)
	assert.Equal(t, "app/ui.js", proxyPath)
}

func TestJoinURLPath(t *testing.T) {
	assert.Equal(t, "/internal/ref/stream/vnc.html", joinURLPath("/internal/ref/stream", "vnc.html"))
	assert.Equal(t, "/vnc.html", joinURLPath("/", "vnc.html"))
	assert.Equal(t, "/internal/ref/stream", joinURLPath("/internal/ref/stream", ""))
}

func TestBrowserWorkerInternalHeadersUsesInternalToken(t *testing.T) {
	t.Setenv(browserWorkerInternalTokenEnv, "worker-token")

	headers := browserWorkerInternalHeaders()

	assert.Equal(t, "Bearer worker-token", headers.Get(echo.HeaderAuthorization))
}

func TestBrowserWorkerInternalHeadersClearsAuthorizationWhenTokenMissing(t *testing.T) {
	t.Setenv(browserWorkerInternalTokenEnv, "")

	headers := browserWorkerInternalHeaders()
	outgoing := http.Header{
		echo.HeaderAuthorization: []string{"Bearer user-token"},
	}
	for key, values := range headers {
		outgoing.Del(key)
		for _, value := range values {
			outgoing.Add(key, value)
		}
	}

	assert.Empty(t, outgoing.Get(echo.HeaderAuthorization))
}

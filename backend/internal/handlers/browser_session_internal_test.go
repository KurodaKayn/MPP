package handlers

import (
	"testing"

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

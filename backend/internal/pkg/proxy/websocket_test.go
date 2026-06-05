package proxy

import (
	"bytes"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSocket(t *testing.T) {
	// 1. Setup a real WebSocket server to act as the target (e.g., the worker/container)
	upgrader := websocket.Upgrader{}
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			mt, message, err := conn.ReadMessage()
			if err != nil {
				break
			}
			err = conn.WriteMessage(mt, message)
			if err != nil {
				break
			}
		}
	}))
	defer targetServer.Close()

	targetURL, _ := url.Parse(targetServer.URL)

	// 2. Setup Echo server with our WebSocket handler
	e := echo.New()
	e.GET("/ws", func(c echo.Context) error {
		return WebSocket(c, targetURL)
	})

	proxyServer := httptest.NewServer(e)
	defer proxyServer.Close()

	// 3. Connect to the proxy server with a WebSocket client
	proxyWSURL := strings.Replace(proxyServer.URL, "http", "ws", 1) + "/ws"

	dialer := websocket.DefaultDialer
	conn, resp, err := dialer.Dial(proxyWSURL, nil)
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	defer func() { _ = conn.Close() }()
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	// 4. Test data transmission
	message := []byte("hello websocket")
	err = conn.WriteMessage(websocket.TextMessage, message)
	require.NoError(t, err)

	_, p, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, message, p)

	// 5. Test timeout/closure
	require.NoError(t, conn.SetWriteDeadline(time.Now().Add(time.Second)))
	err = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	require.NoError(t, err)
}

func TestWebSocketProxyConfigFromEnv(t *testing.T) {
	t.Setenv(webSocketDialTimeoutEnv, "2s")
	t.Setenv(webSocketIdleTimeoutEnv, "3")
	t.Setenv(webSocketMaxConnectionTimeEnv, "bad")

	config := webSocketProxyConfigFromEnv()

	require.Equal(t, 2*time.Second, config.DialTimeout)
	require.Equal(t, 3*time.Second, config.IdleTimeout)
	require.Equal(t, defaultWebSocketMaxLifetime, config.MaxConnectionTime)
}

func TestDeadlineReaderAllowsNilConn(t *testing.T) {
	reader := deadlineReader{
		Reader:  bytes.NewBufferString("hello"),
		Timeout: time.Millisecond,
	}
	buffer := make([]byte, 5)

	n, err := reader.Read(buffer)

	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, "hello", string(buffer))
}

func TestDeadlineReaderSetsReadDeadline(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	defer func() { _ = server.Close() }()

	reader := deadlineReader{
		Reader:  client,
		Conn:    client,
		Timeout: 5 * time.Millisecond,
	}
	buffer := make([]byte, 1)

	_, err := reader.Read(buffer)

	require.Error(t, err)
	var netErr net.Error
	ok := errors.As(err, &netErr)
	require.True(t, ok)
	require.True(t, netErr.Timeout())
}

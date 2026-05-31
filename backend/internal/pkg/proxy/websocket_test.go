package proxy

import (
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

func TestProxyWebSocket(t *testing.T) {
	// 1. Setup a real WebSocket server to act as the target (e.g., the worker/container)
	upgrader := websocket.Upgrader{}
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
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

	// 2. Setup Echo server with our ProxyWebSocket handler
	e := echo.New()
	e.GET("/ws", func(c echo.Context) error {
		return ProxyWebSocket(c, targetURL)
	})

	proxyServer := httptest.NewServer(e)
	defer proxyServer.Close()

	// 3. Connect to the proxy server with a WebSocket client
	proxyWSURL := strings.Replace(proxyServer.URL, "http", "ws", 1) + "/ws"
	
	dialer := websocket.DefaultDialer
	conn, resp, err := dialer.Dial(proxyWSURL, nil)
	require.NoError(t, err)
	defer conn.Close()
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	// 4. Test data transmission
	message := []byte("hello websocket")
	err = conn.WriteMessage(websocket.TextMessage, message)
	assert.NoError(t, err)

	_, p, err := conn.ReadMessage()
	assert.NoError(t, err)
	assert.Equal(t, message, p)

	// 5. Test timeout/closure
	conn.SetWriteDeadline(time.Now().Add(time.Second))
	err = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	assert.NoError(t, err)
}

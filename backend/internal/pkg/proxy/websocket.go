package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/kurodakayn/mpp-backend/internal/pkg/envutil"
)

const (
	webSocketDialTimeoutEnv       = "WEBSOCKET_PROXY_DIAL_TIMEOUT"
	webSocketIdleTimeoutEnv       = "WEBSOCKET_PROXY_IDLE_TIMEOUT"
	webSocketMaxConnectionTimeEnv = "WEBSOCKET_PROXY_MAX_CONNECTION_TIME"
	defaultWebSocketDialTimeout   = 10 * time.Second
	defaultWebSocketIdleTimeout   = 75 * time.Second
	defaultWebSocketMaxLifetime   = 16 * time.Minute
)

var webSocketTLSConfig = func(target *url.URL) *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: target.Hostname(),
	}
}

// WebSocket hijacks the echo context connection and pipes it to the target URL
func WebSocket(c echo.Context, target *url.URL) error {
	return WebSocketWithHeaders(c, target, nil)
}

// WebSocketWithHeaders hijacks the echo context connection and sends extra headers to the target URL.
func WebSocketWithHeaders(c echo.Context, target *url.URL, headers http.Header) error {
	req := c.Request()
	res := c.Response()
	config := webSocketProxyConfigFromEnv()

	// 1. Setup connection to target
	targetAddr := target.Host
	if !strings.Contains(targetAddr, ":") {
		if target.Scheme == "https" || target.Scheme == "wss" {
			targetAddr += ":443"
		} else {
			targetAddr += ":80"
		}
	}

	targetConn, err := dialWebSocketTarget(req.Context(), target, targetAddr, config.DialTimeout)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "failed to connect to stream target")
	}
	defer func() { _ = targetConn.Close() }()

	// 2. Perform handshake with target
	// We need to send the original request but update headers
	targetReq, err := http.NewRequestWithContext(req.Context(), req.Method, target.String(), nil)
	if err != nil {
		return err
	}

	for k, vv := range req.Header {
		for _, v := range vv {
			targetReq.Header.Add(k, v)
		}
	}
	for k, vv := range headers {
		targetReq.Header.Del(k)
		for _, v := range vv {
			targetReq.Header.Add(k, v)
		}
	}
	// Ensure Host is correct
	targetReq.Host = target.Host

	if err := targetReq.Write(targetConn); err != nil {
		return err
	}

	// 3. Hijack client connection
	hijacker, ok := res.Writer.(http.Hijacker)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "webserver does not support hijacking")
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		return err
	}
	defer func() { _ = clientConn.Close() }()

	if config.MaxConnectionTime > 0 {
		_ = targetConn.SetDeadline(time.Now().Add(config.MaxConnectionTime))
		_ = clientConn.SetDeadline(time.Now().Add(config.MaxConnectionTime))
	}

	// 4. Pipe data
	errChan := make(chan error, 2)
	cp := func(dst io.Writer, src io.Reader) {
		if config.IdleTimeout > 0 {
			src = deadlineReader{Reader: src, Conn: deadlineConn(src), Timeout: config.IdleTimeout}
		}
		_, err := io.Copy(dst, src)
		errChan <- err
	}

	go cp(targetConn, clientConn)
	go cp(clientConn, targetConn)

	select {
	case <-req.Context().Done():
		return req.Context().Err()
	case err := <-errChan:
		if err != nil && !errors.Is(err, io.EOF) {
			log.Printf("WebSocket proxy error: %v", err)
		}
		return nil
	}
}

func dialWebSocketTarget(ctx context.Context, target *url.URL, targetAddr string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", targetAddr)
	if err != nil {
		return nil, err
	}

	if target.Scheme != "https" && target.Scheme != "wss" {
		return conn, nil
	}

	tlsConn := tls.Client(conn, webSocketTLSConfig(target))
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return tlsConn, nil
}

type webSocketProxyConfig struct {
	DialTimeout       time.Duration
	IdleTimeout       time.Duration
	MaxConnectionTime time.Duration
}

func webSocketProxyConfigFromEnv() webSocketProxyConfig {
	return webSocketProxyConfig{
		DialTimeout:       envutil.Duration(webSocketDialTimeoutEnv, defaultWebSocketDialTimeout),
		IdleTimeout:       envutil.Duration(webSocketIdleTimeoutEnv, defaultWebSocketIdleTimeout),
		MaxConnectionTime: envutil.Duration(webSocketMaxConnectionTimeEnv, defaultWebSocketMaxLifetime),
	}
}

type deadlineReader struct {
	io.Reader
	Conn    net.Conn
	Timeout time.Duration
}

func (r deadlineReader) Read(p []byte) (int, error) {
	if r.Conn != nil && r.Timeout > 0 {
		_ = r.Conn.SetReadDeadline(time.Now().Add(r.Timeout))
	}
	return r.Reader.Read(p)
}

func deadlineConn(reader io.Reader) net.Conn {
	if conn, ok := reader.(net.Conn); ok {
		return conn
	}
	return nil
}

// TransparentProxy wraps Echo's ReverseProxy but handles WebSockets
func TransparentProxy(target *url.URL) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if strings.ToLower(c.Request().Header.Get("Upgrade")) == "websocket" {
				return WebSocket(c, target)
			}
			return next(c)
		}
	}
}

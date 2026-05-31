package proxy

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"
)

// ProxyWebSocket hijacks the echo context connection and pipes it to the target URL
func ProxyWebSocket(c echo.Context, target *url.URL) error {
	req := c.Request()
	res := c.Response()

	// 1. Setup connection to target
	targetAddr := target.Host
	if !strings.Contains(targetAddr, ":") {
		if target.Scheme == "https" || target.Scheme == "wss" {
			targetAddr += ":443"
		} else {
			targetAddr += ":80"
		}
	}

	d := net.Dialer{}
	targetConn, err := d.DialContext(req.Context(), "tcp", targetAddr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "failed to connect to stream target")
	}
	defer targetConn.Close()

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
	defer clientConn.Close()

	// 4. Pipe data
	errChan := make(chan error, 2)
	cp := func(dst io.Writer, src io.Reader) {
		_, err := io.Copy(dst, src)
		errChan <- err
	}

	go cp(targetConn, clientConn)
	go cp(clientConn, targetConn)

	select {
	case <-req.Context().Done():
		return req.Context().Err()
	case err := <-errChan:
		if err != nil && err != io.EOF {
			log.Printf("WebSocket proxy error: %v", err)
		}
		return nil
	}
}

// TransparentProxy wraps Echo's ReverseProxy but handles WebSockets
func TransparentProxy(target *url.URL) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if strings.ToLower(c.Request().Header.Get("Upgrade")) == "websocket" {
				return ProxyWebSocket(c, target)
			}
			return next(c)
		}
	}
}

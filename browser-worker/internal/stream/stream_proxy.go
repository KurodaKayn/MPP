package stream

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/kurodakayn/mpp-browser-worker/internal/session"
)

func Handler(sm *session.Manager) echo.HandlerFunc {
	return func(c echo.Context) error {
		ref := c.Param("ref")
		workerSession, ok := sm.Get(ref)
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}

		targetURL, err := url.Parse(workerSession.InternalStreamURL)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "invalid stream endpoint")
		}

		if strings.ToLower(c.Request().Header.Get("Upgrade")) == "websocket" {
			subPath := c.Param("*")
			if subPath != "" {
				targetURL.Path = workerStreamPath(subPath)
			}
			return proxyWebSocket(c, targetURL)
		}

		proxy := &httputil.ReverseProxy{}
		proxy.Rewrite = func(req *httputil.ProxyRequest) {
			req.SetURL(targetURL)
			req.Out.URL.Path = workerStreamPath(c.Param("*"))
			req.Out.URL.RawQuery = req.In.URL.RawQuery
			req.Out.Host = targetURL.Host
		}
		proxy.ServeHTTP(c.Response(), c.Request())
		return nil
	}
}

func proxyWebSocket(c echo.Context, target *url.URL) error {
	req := c.Request()
	res := c.Response()

	targetAddr := target.Host
	if !strings.Contains(targetAddr, ":") {
		targetAddr += ":80"
	}

	d := net.Dialer{}
	targetConn, err := d.DialContext(req.Context(), "tcp", targetAddr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "failed to connect to stream target")
	}
	defer func() { _ = targetConn.Close() }()

	targetReq, err := http.NewRequestWithContext(req.Context(), req.Method, target.String(), nil)
	if err != nil {
		return err
	}
	for k, vv := range req.Header {
		for _, v := range vv {
			targetReq.Header.Add(k, v)
		}
	}
	targetReq.Host = target.Host

	if err := targetReq.Write(targetConn); err != nil {
		return err
	}

	hijacker, ok := res.Writer.(http.Hijacker)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "webserver does not support hijacking")
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		return err
	}
	defer func() { _ = clientConn.Close() }()

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
		if err != nil && !errors.Is(err, io.EOF) {
			log.Printf("WebSocket worker proxy error: %v", err)
		}
		return nil
	}
}

func endpointPort(endpoint string) (int, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return 0, err
	}
	var port int
	if _, err := fmt.Sscanf(u.Port(), "%d", &port); err != nil {
		return 0, fmt.Errorf("missing endpoint port")
	}
	return port, nil
}

func workerStreamPath(wildcardPath string) string {
	wildcardPath = strings.TrimPrefix(wildcardPath, "/")
	if wildcardPath == "" {
		return "/"
	}
	return "/" + wildcardPath
}

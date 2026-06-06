package handlers

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/pkg/envutil"
	"github.com/kurodakayn/mpp-backend/internal/pkg/proxy"
	"github.com/kurodakayn/mpp-backend/internal/pkg/streamgate"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
)

const (
	browserStreamGatewayTimeoutEnv     = "BROWSER_STREAM_GATEWAY_TIMEOUT"
	browserWorkerInternalTokenEnv      = "BROWSER_WORKER_INTERNAL_TOKEN"
	defaultBrowserStreamGatewayTimeout = 15 * time.Second
)

var browserStreamTransport = newBrowserStreamTransportFromEnv()

type BrowserSessionHandler struct {
	service       *browsersession.BrowserSessionService
	streamLimiter *streamgate.Limiter
}

func NewBrowserSessionHandler(service *browsersession.BrowserSessionService) *BrowserSessionHandler {
	return &BrowserSessionHandler{service: service}
}

func (h *BrowserSessionHandler) UseStreamLimiter(limiter *streamgate.Limiter) {
	h.streamLimiter = limiter
}

func (h *BrowserSessionHandler) StartSession(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	platform := c.Param("platform")
	if platform == "" {
		return sendError(c, http.StatusBadRequest, "invalid_request", "platform is required")
	}

	tenantID, err := middleware.GetTenantIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	resp, err := h.service.StartSessionForTenant(c.Request().Context(), userID, tenantID, platform)
	if err != nil {
		if errors.Is(err, browsersession.ErrActiveSessionExists) {
			return sendError(c, http.StatusConflict, "conflict", err.Error())
		}
		if errors.Is(err, browsersession.ErrUserQuotaExceeded) || errors.Is(err, browsersession.ErrTenantQuotaExceeded) {
			return sendError(c, http.StatusTooManyRequests, "quota_exceeded", err.Error())
		}
		if errors.Is(err, browsersession.ErrWorkerPoolExhausted) {
			return sendError(c, http.StatusServiceUnavailable, "worker_pool_exhausted", err.Error())
		}
		if errors.Is(err, browsersession.ErrPlatformNotSupported) {
			return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusCreated, resp)
}

func (h *BrowserSessionHandler) GetSession(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid session id")
	}

	resp, err := h.service.GetSession(c.Request().Context(), userID, id)
	if err != nil {
		if errors.Is(err, browsersession.ErrSessionNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", err.Error())
		}
		if errors.Is(err, browsersession.ErrSessionGone) {
			return sendError(c, http.StatusGone, "gone", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *BrowserSessionHandler) StreamSession(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid session id")
	}

	streamToken, proxyPath := streamTokenAndProxyPath(c.QueryParam("token"), c.Param("*"))
	log.Printf("StreamSession: id=%s, path=%s", id, proxyPath)

	isWebSocket := strings.ToLower(c.Request().Header.Get("Upgrade")) == "websocket"
	tenantID, err := middleware.GetTenantIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	var lease *streamgate.Lease
	if shouldAcquireBrowserStreamLease(c, isWebSocket, proxyPath) {
		lease, err = h.acquireBrowserStreamLease(c, userID, tenantID, id)
		if err != nil {
			if handled := streamgate.SendLimitError(c, err); handled != nil {
				return handled
			}
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}
		defer func() { _ = lease.Release(context.Background()) }()
	}

	endpoint, err := h.service.GetStreamEndpoint(c.Request().Context(), userID, id, streamToken, isWebSocket)
	if err != nil {
		if errors.Is(err, browsersession.ErrSessionNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", err.Error())
		}
		if errors.Is(err, browsersession.ErrSessionForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		if errors.Is(err, browsersession.ErrStreamTokenGone) {
			return sendError(c, http.StatusGone, "gone", err.Error())
		}
		if errors.Is(err, browsersession.ErrInvalidStreamToken) {
			return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	target, err := url.Parse(endpoint)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", "invalid stream endpoint")
	}
	target.Scheme = reverseProxyScheme(target.Scheme)
	rawQuery := streamProxyRawQuery(c)
	internalHeaders := browserWorkerInternalHeaders()

	// Use custom WebSocket proxy for noVNC stream (e.g. when requesting /websockify)
	if isWebSocket {
		// Update target path with proxy path before proxying
		target.Path = joinURLPath(target.Path, proxyPath)
		target.RawQuery = rawQuery
		return proxy.WebSocketWithHeaders(c, target, internalHeaders)
	}

	reverseProxy := &httputil.ReverseProxy{
		Transport: browserStreamTransport,
		Rewrite: func(proxyRequest *httputil.ProxyRequest) {
			req := proxyRequest.Out
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = joinURLPath(target.Path, proxyPath)
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
			req.URL.RawQuery = rawQuery
			req.Host = target.Host
			for key, values := range internalHeaders {
				req.Header.Del(key)
				for _, value := range values {
					req.Header.Add(key, value)
				}
			}
			proxyRequest.SetXForwarded()
		},
	}
	reverseProxy.ErrorHandler = browserStreamProxyErrorHandler
	c.Response().Header().Set("X-MPP-Stream-Reconnect", "true")
	if lease != nil && lease.ID != "" {
		c.Response().Header().Set("X-MPP-Stream-ID", lease.ID)
	}
	reverseProxy.ServeHTTP(c.Response(), c.Request())
	return nil
}

func browserWorkerInternalHeaders() http.Header {
	headers := http.Header{
		echo.HeaderAuthorization: []string{},
	}
	token := strings.TrimSpace(os.Getenv(browserWorkerInternalTokenEnv))
	if token == "" {
		return headers
	}
	headers.Set(echo.HeaderAuthorization, "Bearer "+token)
	return headers
}

func shouldAcquireBrowserStreamLease(c echo.Context, isWebSocket bool, proxyPath string) bool {
	if isWebSocket {
		return true
	}
	path := strings.ToLower(proxyPath)
	queryPath := strings.ToLower(c.QueryParam("path"))
	return strings.Contains(path, "websockify") || strings.Contains(queryPath, "websockify")
}

func newBrowserStreamTransportFromEnv() http.RoundTripper {
	timeout := envutil.Duration(browserStreamGatewayTimeoutEnv, defaultBrowserStreamGatewayTimeout)
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func browserStreamProxyErrorHandler(rw http.ResponseWriter, _ *http.Request, err error) {
	status := http.StatusBadGateway
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) && netErr.Timeout() {
		status = http.StatusGatewayTimeout
	}
	http.Error(rw, http.StatusText(status), status)
}

func (h *BrowserSessionHandler) acquireBrowserStreamLease(c echo.Context, userID uuid.UUID, tenantID string, sessionID uuid.UUID) (*streamgate.Lease, error) {
	if h.streamLimiter == nil {
		return &streamgate.Lease{}, nil
	}
	return h.streamLimiter.Acquire(c.Request().Context(), streamgate.AcquireRequest{
		Kind:     streamgate.KindBrowser,
		UserID:   userID,
		TenantID: tenantID,
		IP:       streamgate.ClientIP(c),
		Resource: sessionID.String(),
	})
}

func (h *BrowserSessionHandler) CompleteSession(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid session id")
	}

	resp, err := h.service.CompleteSession(c.Request().Context(), userID, id)
	if err != nil {
		if errors.Is(err, browsersession.ErrSessionNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", err.Error())
		}
		if errors.Is(err, browsersession.ErrLoginNotDetected) || errors.Is(err, browsersession.ErrSessionNotReady) {
			return sendError(c, http.StatusUnprocessableEntity, "unprocessable_entity", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *BrowserSessionHandler) CancelSession(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid session id")
	}

	if err := h.service.CancelSession(c.Request().Context(), userID, id); err != nil {
		if errors.Is(err, browsersession.ErrSessionNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, dto.CancelBrowserSessionResponse{
		SessionID: id,
		Status:    "expired",
	})
}

func streamTokenAndProxyPath(queryToken string, wildcardPath string) (string, string) {
	wildcardPath = strings.TrimPrefix(wildcardPath, "/")
	if queryToken != "" {
		return queryToken, wildcardPath
	}

	token, proxyPath, hasProxyPath := strings.Cut(wildcardPath, "/")
	if !hasProxyPath {
		return token, ""
	}
	return token, proxyPath
}

func streamProxyRawQuery(c echo.Context) string {
	values := c.QueryParams()
	values.Del("token")
	return values.Encode()
}

func reverseProxyScheme(scheme string) string {
	switch scheme {
	case "ws":
		return "http"
	case "wss":
		return "https"
	default:
		return scheme
	}
}

func joinURLPath(base string, suffix string) string {
	if suffix == "" {
		if base == "" {
			return "/"
		}
		return base
	}
	if base == "" || base == "/" {
		return "/" + strings.TrimPrefix(suffix, "/")
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimPrefix(suffix, "/")
}

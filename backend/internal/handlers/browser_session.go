package handlers

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/pkg/proxy"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	"github.com/labstack/echo/v4"
)

type BrowserSessionHandler struct {
	service *browsersession.BrowserSessionService
}

func NewBrowserSessionHandler(service *browsersession.BrowserSessionService) *BrowserSessionHandler {
	return &BrowserSessionHandler{service: service}
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

	resp, err := h.service.StartSession(c.Request().Context(), userID, platform)
	if err != nil {
		if err == browsersession.ErrActiveSessionExists {
			return sendError(c, http.StatusConflict, "conflict", err.Error())
		}
		if err == browsersession.ErrPlatformNotSupported {
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
		if err == browsersession.ErrSessionNotFound {
			return sendError(c, http.StatusNotFound, "not_found", err.Error())
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
	isWebSocket := strings.ToLower(c.Request().Header.Get("Upgrade")) == "websocket"
	endpoint, err := h.service.GetStreamEndpoint(c.Request().Context(), userID, id, streamToken, false)
	if err != nil {
		if err == browsersession.ErrSessionNotFound {
			return sendError(c, http.StatusNotFound, "not_found", err.Error())
		}
		if err == browsersession.ErrSessionForbidden {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		if err == browsersession.ErrStreamTokenGone {
			return sendError(c, http.StatusGone, "gone", err.Error())
		}
		if err == browsersession.ErrInvalidStreamToken {
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

	// Use custom WebSocket proxy for noVNC stream (e.g. when requesting /websockify)
	if isWebSocket {
		// Update target path with proxy path before proxying
		target.Path = joinURLPath(target.Path, proxyPath)
		target.RawQuery = rawQuery
		return proxy.ProxyWebSocket(c, target)
	}

	reverseProxy := httputil.NewSingleHostReverseProxy(target)
	reverseProxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = joinURLPath(target.Path, proxyPath)
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}
		req.URL.RawQuery = rawQuery
		req.Host = target.Host
	}
	reverseProxy.ServeHTTP(c.Response(), c.Request())
	return nil
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
		if err == browsersession.ErrSessionNotFound {
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

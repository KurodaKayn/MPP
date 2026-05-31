package handlers

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/pkg/proxy"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/labstack/echo/v4"
)

type BrowserSessionHandler struct {
	service *services.BrowserSessionService
}

func NewBrowserSessionHandler(service *services.BrowserSessionService) *BrowserSessionHandler {
	return &BrowserSessionHandler{service: service}
}

func (h *BrowserSessionHandler) StartSession(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	platform := c.Param("platform")

	resp, err := h.service.StartSession(c.Request().Context(), userID, platform)
	if err != nil {
		if err == services.ErrActiveSessionExists {
			return sendError(c, http.StatusConflict, "conflict", err.Error())
		}
		if err == services.ErrPlatformNotSupported {
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
		if err == services.ErrSessionNotFound {
			return sendError(c, http.StatusNotFound, "not_found", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *BrowserSessionHandler) StreamSession(c echo.Context) error {
	// For streaming, we support two auth methods:
	// 1. JWT (normal user session)
	// 2. Stream Token (for direct browser/iframe access where headers can't be set)
	
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid session id")
	}

	token := c.QueryParam("token")
	userID, _ := middleware.GetUserIDFromContext(c) // May be nil if coming from public route

	// The service will validate the token against the session, and optionally the userID if provided
	endpoint, err := h.service.GetStreamEndpoint(c.Request().Context(), userID, id, token)
	if err != nil {
		if err == services.ErrSessionNotFound {
			return sendError(c, http.StatusNotFound, "not_found", err.Error())
		}
		if err == services.ErrInvalidStreamToken {
			return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	target, err := url.Parse(endpoint)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", "invalid stream endpoint")
	}

	// Use custom WebSocket proxy for noVNC stream
	if strings.ToLower(c.Request().Header.Get("Upgrade")) == "websocket" {
		return proxy.ProxyWebSocket(c, target)
	}

	// Fallback to regular proxy for assets (vnc.html, js, css)
	p := httputil.NewSingleHostReverseProxy(target)
	originalDirector := p.Director
	p.Director = func(req *http.Request) {
		originalDirector(req)
		// Extract the subpath after "/stream"
		// e.g., /api/user/dashboard/browser-sessions/id/stream/vnc.html -> /vnc.html
		path := c.Request().URL.Path
		if idx := strings.Index(path, "/stream"); idx != -1 {
			subPath := path[idx+len("/stream"):]
			if subPath == "" || subPath == "/" {
				req.URL.Path = target.Path + "/"
			} else {
				req.URL.Path = target.Path + subPath
			}
		}
	}
	p.ServeHTTP(c.Response(), c.Request())
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
		if err == services.ErrSessionNotFound {
			return sendError(c, http.StatusNotFound, "not_found", err.Error())
		}
		// Assuming 422 for login not detected yet as per design
		return sendError(c, http.StatusUnprocessableEntity, "unprocessable_entity", err.Error())
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
		if err == services.ErrSessionNotFound {
			return sendError(c, http.StatusNotFound, "not_found", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

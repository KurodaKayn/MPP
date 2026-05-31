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
	
	// If token is missing in query, try to get it from a session-specific cookie
	// This allows loading sub-resources (js/css) which don't have the token in the URL
	cookieName := "mpp_st_" + id.String()
	if token == "" {
		if cookie, err := c.Cookie(cookieName); err == nil {
			token = cookie.Value
		}
	}

	userID, _ := middleware.GetUserIDFromContext(c)

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

	// If we had a valid token in the query, set/refresh the session cookie
	if c.QueryParam("token") != "" {
		c.SetCookie(&http.Cookie{
			Name:     cookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   3600, // 1 hour
		})
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
		// Use the wildcard parameter from Echo to get the sub-path
		subPath := c.Param("*")
		
		// Ensure target path ends without trailing slash for consistent joining
		targetPath := strings.TrimSuffix(target.Path, "/")
		
		if subPath == "" || subPath == "/" {
			req.URL.Path = targetPath + "/vnc.html"
		} else {
			if !strings.HasPrefix(subPath, "/") {
				subPath = "/" + subPath
			}
			req.URL.Path = targetPath + subPath
		}
		// Ensure the Host header and other proxy headers are set correctly
		req.Host = target.Host
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

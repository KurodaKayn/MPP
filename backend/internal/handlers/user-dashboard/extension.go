package userdashboard

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
	extensionsvc "github.com/kurodakayn/mpp-backend/internal/services/extension"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
)

func (h *Handler) GetExtensionSession(c echo.Context) error {
	return h.withAuthenticatedDashboardRequest(c, func(dashReq *dashboardRequest) error {
		session, err := dashReq.service.GetExtensionSession(dashReq.userID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return sendError(c, http.StatusUnauthorized, "unauthorized", "session user not found")
			}
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusOK, session)
	})
}

func (h *Handler) ListExtensionPrepublish(c echo.Context) error {
	return h.withAuthenticatedDashboardRequest(c, func(dashReq *dashboardRequest) error {
		resp, err := dashReq.service.ListExtensionPrepublish(dashReq.userID)
		if err != nil {
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusOK, resp)
	})
}

func (h *Handler) CreateExtensionHandoff(c echo.Context) error {
	return h.withAuthenticatedDashboardRequest(c, func(dashReq *dashboardRequest) error {
		req := new(dto.CreateExtensionHandoffRequest)
		if err := dashReq.bind(req); err != nil {
			return err
		}

		handoff, err := dashReq.service.CreateExtensionHandoff(dashReq.userID, *req, extensionEventsCallbackURL(c))
		if err != nil {
			if errors.Is(err, projectsvc.ErrInvalidProject) {
				return sendError(c, http.StatusBadRequest, "invalid_request", "project_id and supported platforms are required")
			}
			if errors.Is(err, publishsvc.ErrPublicationDisabled) {
				return sendError(c, http.StatusBadRequest, "invalid_request", "publication is disabled for this project")
			}
			if errors.Is(err, publishsvc.ErrPublicationRequiresSync) {
				return sendError(c, http.StatusBadRequest, "invalid_request", "sync prepublish draft before extension handoff")
			}
			if errors.Is(err, accesspolicy.ErrForbidden) {
				return sendError(c, http.StatusForbidden, "forbidden", err.Error())
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return sendError(c, http.StatusNotFound, "not_found", "project not found")
			}
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusOK, handoff)
	})
}

func (h *Handler) RecordExtensionEvent(c echo.Context) error {
	req := new(dto.ExtensionEventCallbackRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	resp, err := h.dashboardService.RecordExtensionEvent(*req)
	if err != nil {
		if errors.Is(err, extensionsvc.ErrExtensionCallbackTokenInvalid) ||
			errors.Is(err, extensionsvc.ErrExtensionCallbackTokenExpired) {
			return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func extensionEventsCallbackURL(c echo.Context) string {
	proto := strings.TrimSpace(c.Request().Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		if c.Request().TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}

	host := strings.TrimSpace(c.Request().Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = c.Request().Host
	}
	return proto + "://" + host + "/api/user/dashboard/extension/events"
}

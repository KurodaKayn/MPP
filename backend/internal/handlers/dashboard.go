package handlers

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
	dashboardsvc "github.com/kurodakayn/mpp-backend/internal/services/dashboard"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
	readmodelsvc "github.com/kurodakayn/mpp-backend/internal/services/readmodel"
)

type DashboardHandler struct {
	dashboardService *dashboardsvc.DashboardService
}

func NewDashboardHandler(s *dashboardsvc.DashboardService) *DashboardHandler {
	return &DashboardHandler{dashboardService: s}
}

func (h *DashboardHandler) serviceFor(c echo.Context) *dashboardsvc.DashboardService {
	return h.dashboardService.WithContext(c.Request().Context())
}

func (h *DashboardHandler) GetStats(c echo.Context) error {
	// Admin view: no scope
	stats, err := h.serviceFor(c).GetStats(nil)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
	return c.JSON(http.StatusOK, stats)
}

func (h *DashboardHandler) ListProjects(c echo.Context) error {
	page, limit := projectPaginationFromQuery(c)
	cursor := c.QueryParam("cursor")
	status := c.QueryParam("status")
	userID := c.QueryParam("user_id")
	platform := c.QueryParam("platform")

	// Admin view: no scope, filterUserID allowed
	resp, err := h.serviceFor(c).ListProjectsCursor(cursor, page, limit, status, userID, platform, nil)
	if err != nil {
		if errors.Is(err, projectsvc.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *DashboardHandler) GetProjectPublications(c echo.Context) error {
	idParam := c.Param("id")
	projectID, err := uuid.Parse(idParam)
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}

	// Admin view: no scope
	includeContent := c.QueryParam("include_content") == "true"
	resp, err := h.serviceFor(c).GetProjectPublications(projectID, nil, includeContent)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *DashboardHandler) RebuildReadModels(c echo.Context) error {
	info, err := h.serviceFor(c).EnqueueDashboardReadModelRebuild(c.Request().Context())
	if err != nil {
		if errors.Is(err, readmodelsvc.ErrDashboardRebuildQueueUnavailable) {
			return sendError(c, http.StatusServiceUnavailable, "queue_unavailable", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
	return c.JSON(http.StatusAccepted, info)
}

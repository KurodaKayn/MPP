package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type DashboardHandler struct {
	dashboardService *services.DashboardService
}

func NewDashboardHandler(s *services.DashboardService) *DashboardHandler {
	return &DashboardHandler{dashboardService: s}
}

func sendError(c echo.Context, code int, errCode, message string) error {
	resp := dto.ErrorResponse{}
	resp.Error.Code = errCode
	resp.Error.Message = message
	return c.JSON(code, resp)
}

func (h *DashboardHandler) GetStats(c echo.Context) error {
	// Admin view: no scope
	stats, err := h.dashboardService.GetStats(nil)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
	return c.JSON(http.StatusOK, stats)
}

func (h *DashboardHandler) ListProjects(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	status := c.QueryParam("status")
	userID := c.QueryParam("user_id")
	platform := c.QueryParam("platform")

	// Admin view: no scope, filterUserID allowed
	resp, err := h.dashboardService.ListProjects(page, limit, status, userID, platform, nil)
	if err != nil {
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
	resp, err := h.dashboardService.GetProjectPublications(projectID, nil, includeContent)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, services.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

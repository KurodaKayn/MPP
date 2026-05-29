package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/middleware"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/services"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type UserDashboardHandler struct {
	dashboardService *services.DashboardService
}

func NewUserDashboardHandler(s *services.DashboardService) *UserDashboardHandler {
	return &UserDashboardHandler{dashboardService: s}
}

func (h *UserDashboardHandler) GetMyStats(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	stats, err := h.dashboardService.GetStats(&userID)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
	return c.JSON(http.StatusOK, stats)
}

func (h *UserDashboardHandler) ListMyProjects(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

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
	platform := c.QueryParam("platform")

	// Personal view: enforce scopeUserID, ignore any requested filterUserID
	resp, err := h.dashboardService.ListProjects(page, limit, status, "", platform, &userID)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) GetMyProjectPublications(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	idParam := c.Param("id")
	projectID, err := uuid.Parse(idParam)
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}

	// Personal view: enforce scopeUserID to check ownership
	resp, err := h.dashboardService.GetProjectPublications(projectID, &userID)
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

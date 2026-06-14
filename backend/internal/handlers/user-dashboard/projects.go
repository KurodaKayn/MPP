package userdashboard

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
)

func (h *Handler) ListMyProjects(c echo.Context) error {
	return h.withAuthenticatedDashboardRequest(c, func(dashReq *dashboardRequest) error {
		page, limit := projectPaginationFromQuery(c)
		cursor := c.QueryParam("cursor")
		status := c.QueryParam("status")
		platform := c.QueryParam("platform")
		workspaceID, err := dashReq.optionalWorkspaceID()
		if err != nil {
			return err
		}

		if workspaceID != uuid.Nil {
			resp, err := dashReq.service.ListWorkspaceProjectsCursor(workspaceID, dashReq.userID, cursor, page, limit, status, platform)
			if err != nil {
				return sendWorkspaceError(c, err)
			}
			return c.JSON(http.StatusOK, resp)
		}

		// Personal view: enforce scopeUserID, ignore any requested filterUserID
		resp, err := dashReq.service.ListProjectsCursor(cursor, page, limit, status, "", platform, &dashReq.userID)
		if err != nil {
			if errors.Is(err, projectsvc.ErrInvalidProject) {
				return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
			}
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusOK, resp)
	})
}

func (h *Handler) CreateProject(c echo.Context) error {
	return h.withAuthenticatedDashboardRequest(c, func(dashReq *dashboardRequest) error {
		req := new(dto.CreateProjectRequest)
		if err := dashReq.bind(req); err != nil {
			return err
		}

		workspaceID, err := dashReq.optionalWorkspaceID()
		if err != nil {
			return err
		}
		if workspaceID != uuid.Nil {
			resp, err := dashReq.service.CreateWorkspaceProject(workspaceID, dashReq.userID, *req)
			if err != nil {
				if errors.Is(err, projectsvc.ErrInvalidProject) {
					return sendError(c, http.StatusBadRequest, "invalid_request", "title, source_content and platforms are required")
				}
				return sendWorkspaceError(c, err)
			}
			return c.JSON(http.StatusCreated, resp)
		}

		resp, err := dashReq.service.CreateProject(dashReq.userID, *req)
		if err != nil {
			if errors.Is(err, projectsvc.ErrInvalidProject) {
				return sendError(c, http.StatusBadRequest, "invalid_request", "title, source_content and platforms are required")
			}
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusCreated, resp)
	})
}

func (h *Handler) GetMyProject(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	idParam := c.Param("id")
	projectID, err := uuid.Parse(idParam)
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	if err := h.ensureProjectWorkspaceContext(c, projectID, userID); err != nil {
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}

	project, err := h.serviceFor(c).GetProject(projectID, &userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, project)
}

func (h *Handler) UpdateProject(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	idParam := c.Param("id")
	projectID, err := uuid.Parse(idParam)
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	if err := h.ensureProjectWorkspaceContext(c, projectID, userID); err != nil {
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendWorkspaceError(c, err)
	}

	req := new(dto.UpdateProjectRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	project, err := h.serviceFor(c).UpdateProject(projectID, userID, *req)
	if err != nil {
		if errors.Is(err, projectsvc.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "title, source_content and platforms are required")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		if errors.Is(err, projectsvc.ErrProjectCollabUnavailable) {
			return sendError(c, http.StatusServiceUnavailable, "service_unavailable", "project collaboration unavailable")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, project)
}

func (h *Handler) DeleteProject(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	if err := h.ensureProjectWorkspaceContext(c, projectID, userID); err != nil {
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}

	if err := h.serviceFor(c).DeleteProject(projectID, userID); err != nil {
		if errors.Is(err, projectsvc.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		if errors.Is(err, projectsvc.ErrProjectDeletionBlocked) {
			return sendError(c, http.StatusConflict, "conflict", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) SaveProjectContent(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	idParam := c.Param("id")
	projectID, err := uuid.Parse(idParam)
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	if err := h.ensureProjectWorkspaceContext(c, projectID, userID); err != nil {
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendWorkspaceError(c, err)
	}

	req := new(dto.SaveProjectContentRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	project, err := h.serviceFor(c).SaveProjectContent(projectID, userID, *req)
	if err != nil {
		if errors.Is(err, projectsvc.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "title and source_content are required")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		if errors.Is(err, projectsvc.ErrProjectCollabUnavailable) {
			return sendError(c, http.StatusServiceUnavailable, "service_unavailable", "project collaboration unavailable")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, project)
}

func (h *Handler) SaveProjectPlatforms(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	idParam := c.Param("id")
	projectID, err := uuid.Parse(idParam)
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	if err := h.ensureProjectWorkspaceContext(c, projectID, userID); err != nil {
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendWorkspaceError(c, err)
	}

	req := new(dto.SaveProjectPlatformsRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	project, err := h.serviceFor(c).SaveProjectPlatforms(projectID, userID, *req)
	if err != nil {
		if errors.Is(err, projectsvc.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "valid platforms are required")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, project)
}

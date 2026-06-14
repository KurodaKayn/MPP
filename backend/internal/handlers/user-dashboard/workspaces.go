package userdashboard

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
)

func (h *Handler) ListWorkspaces(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	resp, err := h.serviceFor(c).ListWorkspaces(userID)
	if err != nil {
		return sendWorkspaceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) CreateWorkspace(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	req := new(dto.CreateWorkspaceRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	workspace, err := h.serviceFor(c).CreateWorkspace(userID, *req)
	if err != nil {
		return sendWorkspaceError(c, err)
	}
	return c.JSON(http.StatusCreated, workspace)
}

func (h *Handler) ListWorkspaceProjects(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}

	page, limit := projectPaginationFromQuery(c)
	cursor := c.QueryParam("cursor")
	status := c.QueryParam("status")
	platform := c.QueryParam("platform")

	resp, err := h.serviceFor(c).ListWorkspaceProjectsCursor(workspaceID, userID, cursor, page, limit, status, platform)
	if err != nil {
		return sendWorkspaceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) CreateWorkspaceProject(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}

	req := new(dto.CreateProjectRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	project, err := h.serviceFor(c).CreateWorkspaceProject(workspaceID, userID, *req)
	if err != nil {
		if errors.Is(err, projectsvc.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "title, source_content and platforms are required")
		}
		return sendWorkspaceError(c, err)
	}
	return c.JSON(http.StatusCreated, project)
}

func (h *Handler) GetWorkspace(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}

	workspace, err := h.serviceFor(c).GetWorkspace(workspaceID, userID)
	if err != nil {
		return sendWorkspaceError(c, err)
	}
	return c.JSON(http.StatusOK, workspace)
}

func (h *Handler) UpdateWorkspace(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}

	req := new(dto.UpdateWorkspaceRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	workspace, err := h.serviceFor(c).UpdateWorkspace(workspaceID, userID, *req)
	if err != nil {
		return sendWorkspaceError(c, err)
	}
	return c.JSON(http.StatusOK, workspace)
}

func (h *Handler) ListWorkspaceMembers(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}

	resp, err := h.serviceFor(c).ListWorkspaceMembers(workspaceID, userID)
	if err != nil {
		return sendWorkspaceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) ListWorkspaceActivities(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	resp, err := h.serviceFor(c).ListWorkspaceActivities(workspaceID, userID, limit)
	if err != nil {
		return sendWorkspaceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) AddWorkspaceMember(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}

	req := new(dto.AddWorkspaceMemberRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	member, err := h.serviceFor(c).AddWorkspaceMember(workspaceID, userID, *req)
	if err != nil {
		return sendWorkspaceError(c, err)
	}
	return c.JSON(http.StatusCreated, member)
}

func (h *Handler) ListWorkspaceInvites(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}

	invites, err := h.serviceFor(c).ListWorkspaceInvites(workspaceID, userID)
	if err != nil {
		return sendWorkspaceError(c, err)
	}
	return c.JSON(http.StatusOK, invites)
}

func (h *Handler) CreateWorkspaceInvite(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}

	req := new(dto.CreateWorkspaceInviteRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	invite, err := h.serviceFor(c).CreateWorkspaceInvite(workspaceID, userID, *req)
	if err != nil {
		return sendWorkspaceError(c, err)
	}
	return c.JSON(http.StatusCreated, invite)
}

func (h *Handler) AcceptWorkspaceInvite(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	req := new(dto.AcceptWorkspaceInviteRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	member, err := h.serviceFor(c).AcceptWorkspaceInvite(userID, *req)
	if err != nil {
		return sendWorkspaceError(c, err)
	}
	return c.JSON(http.StatusOK, member)
}

func (h *Handler) RevokeWorkspaceInvite(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}
	inviteID, err := uuid.Parse(c.Param("inviteId"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid invite UUID")
	}

	if err := h.serviceFor(c).RevokeWorkspaceInvite(workspaceID, userID, inviteID); err != nil {
		return sendWorkspaceError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) UpdateWorkspaceMember(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}
	targetUserID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid user UUID")
	}

	req := new(dto.UpdateWorkspaceMemberRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	member, err := h.serviceFor(c).UpdateWorkspaceMember(workspaceID, userID, targetUserID, *req)
	if err != nil {
		return sendWorkspaceError(c, err)
	}
	return c.JSON(http.StatusOK, member)
}

func (h *Handler) RemoveWorkspaceMember(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}
	targetUserID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid user UUID")
	}

	if err := h.serviceFor(c).RemoveWorkspaceMember(workspaceID, userID, targetUserID); err != nil {
		return sendWorkspaceError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

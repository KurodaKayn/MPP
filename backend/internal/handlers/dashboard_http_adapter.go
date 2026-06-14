package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/services"
)

type dashboardRequest struct {
	c       echo.Context
	service *services.DashboardService
	userID  uuid.UUID
}

func (h *UserDashboardHandler) withAuthenticatedDashboardRequest(c echo.Context, handle func(*dashboardRequest) error) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	return handle(&dashboardRequest{
		c:       c,
		service: h.serviceFor(c),
		userID:  userID,
	})
}

func (h *UserDashboardHandler) withWorkspaceAccountDashboardRequest(c echo.Context, handle func(*dashboardRequest, uuid.UUID) error) error {
	return h.withAuthenticatedDashboardRequest(c, func(req *dashboardRequest) error {
		workspaceID, err := req.optionalWorkspaceID()
		if err != nil {
			return err
		}
		if err := req.requireWorkspaceAccountManager(workspaceID); err != nil {
			return err
		}
		return handle(req, workspaceID)
	})
}

func (r *dashboardRequest) bind(target any) error {
	if err := r.c.Bind(target); err != nil {
		return sendError(r.c, http.StatusBadRequest, "invalid_request", "invalid body")
	}
	return nil
}

func (r *dashboardRequest) optionalWorkspaceID() (uuid.UUID, error) {
	workspaceID, err := workspaceIDFromQuery(r.c)
	if err != nil {
		return uuid.Nil, sendError(r.c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}
	return workspaceID, nil
}

func (r *dashboardRequest) requireWorkspaceAccountManager(workspaceID uuid.UUID) error {
	if workspaceID == uuid.Nil {
		return nil
	}
	_, err := r.service.RequirePermission(workspaceID, r.userID, services.PermissionAccountManage)
	if err != nil {
		return sendWorkspaceError(r.c, err)
	}
	return nil
}

func projectPaginationFromQuery(c echo.Context) (int, int) {
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
	return page, limit
}

func workspaceIDFromQuery(c echo.Context) (uuid.UUID, error) {
	raw := strings.TrimSpace(c.QueryParam("workspace_id"))
	if raw == "" {
		raw = strings.TrimSpace(c.Request().Header.Get("X-Workspace-ID"))
	}
	if raw == "" {
		return uuid.Nil, nil
	}
	return uuid.Parse(raw)
}

func (h *UserDashboardHandler) ensureProjectWorkspaceContext(c echo.Context, projectID uuid.UUID, userID uuid.UUID) error {
	workspaceID, err := workspaceIDFromQuery(c)
	if err != nil || workspaceID == uuid.Nil {
		return err
	}
	project, err := h.serviceFor(c).GetProject(projectID, &userID)
	if err != nil {
		return err
	}
	if project.WorkspaceID == nil || *project.WorkspaceID != workspaceID {
		return services.ErrForbidden
	}
	return nil
}

func parseDashboardTimeParam(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("missing timestamp")
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	return time.Parse("2006-01-02", value)
}

func sendError(c echo.Context, code int, errCode, message string) error {
	resp := dto.ErrorResponse{}
	resp.Error.Code = errCode
	resp.Error.Message = message
	return c.JSON(code, resp)
}

func sendMediaAssetError(c echo.Context, err error) error {
	if errors.Is(err, services.ErrInvalidMediaAsset) || errors.Is(err, services.ErrInvalidProject) {
		return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	if errors.Is(err, services.ErrMediaStorageUnavailable) {
		return sendError(c, http.StatusServiceUnavailable, "media_storage_unavailable", err.Error())
	}
	if errors.Is(err, services.ErrMediaAssetUploadIncomplete) {
		return sendError(c, http.StatusConflict, "upload_incomplete", err.Error())
	}
	if errors.Is(err, services.ErrMediaAssetNotReady) {
		return sendError(c, http.StatusConflict, "media_asset_not_ready", err.Error())
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return sendError(c, http.StatusNotFound, "not_found", "media asset not found")
	}
	if errors.Is(err, services.ErrForbidden) {
		return sendError(c, http.StatusForbidden, "forbidden", err.Error())
	}
	return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
}

func sendProjectCollaboratorError(c echo.Context, err error) error {
	if errors.Is(err, services.ErrInvalidProject) || errors.Is(err, services.ErrInvalidProjectCollaborator) {
		return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return sendError(c, http.StatusNotFound, "not_found", "project collaborator not found")
	}
	if errors.Is(err, services.ErrForbidden) {
		return sendError(c, http.StatusForbidden, "forbidden", err.Error())
	}
	return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
}

func sendProjectExperienceError(c echo.Context, err error) error {
	if errors.Is(err, services.ErrInvalidProject) ||
		errors.Is(err, services.ErrInvalidProjectComment) ||
		errors.Is(err, services.ErrInvalidProjectShareLink) ||
		errors.Is(err, services.ErrInvalidProjectVersion) {
		return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return sendError(c, http.StatusNotFound, "not_found", "project collaboration resource not found")
	}
	if errors.Is(err, services.ErrForbidden) {
		return sendError(c, http.StatusForbidden, "forbidden", err.Error())
	}
	return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
}

func sendWorkspaceError(c echo.Context, err error) error {
	if errors.Is(err, services.ErrInvalidWorkspace) ||
		errors.Is(err, services.ErrInvalidWorkspaceInvite) ||
		errors.Is(err, services.ErrInvalidWorkspaceMember) ||
		errors.Is(err, services.ErrInvalidProject) {
		return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return sendError(c, http.StatusNotFound, "not_found", "workspace resource not found")
	}
	if errors.Is(err, services.ErrForbidden) {
		return sendError(c, http.StatusForbidden, "forbidden", err.Error())
	}
	return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
}

func sendPublishScheduleError(c echo.Context, err error) error {
	if errors.Is(err, services.ErrPublicationDisabled) {
		return sendError(c, http.StatusBadRequest, "invalid_request", "publication is disabled for this project")
	}
	if errors.Is(err, services.ErrPublicationAlreadyPublishing) {
		return sendError(c, http.StatusConflict, "publish_in_progress", "publication is already publishing")
	}
	if errors.Is(err, services.ErrPublicationRequiresSync) {
		return sendError(c, http.StatusBadRequest, "invalid_request", "sync prepublish draft before scheduling")
	}
	if errors.Is(err, services.ErrForbidden) {
		return sendError(c, http.StatusForbidden, "forbidden", err.Error())
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return sendError(c, http.StatusNotFound, "not_found", "schedule not found")
	}
	return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
}

func sendAIEditError(c echo.Context, err error) error {
	if errors.Is(err, services.ErrInvalidAIEditRequest) {
		return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	if errors.Is(err, services.ErrAIServiceUnavailable) {
		return sendError(c, http.StatusBadGateway, "ai_unavailable", err.Error())
	}
	return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
}

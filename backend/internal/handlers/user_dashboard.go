package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/pkg/streamgate"
	"github.com/kurodakayn/mpp-backend/internal/services"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
)

const (
	xOAuth2RedirectURLEnv = "X_OAUTH2_REDIRECT_URL"
	frontendBaseURLEnv    = "FRONTEND_BASE_URL"
)

type UserDashboardHandler struct {
	dashboardService *services.DashboardService
	aiContentEditor  services.AIContentEditor
	streamLimiter    *streamgate.Limiter
}

func NewUserDashboardHandler(s *services.DashboardService) *UserDashboardHandler {
	return &UserDashboardHandler{dashboardService: s}
}

func (h *UserDashboardHandler) serviceFor(c echo.Context) *services.DashboardService {
	return h.dashboardService.WithContext(c.Request().Context())
}

func (h *UserDashboardHandler) UseAIContentEditor(editor services.AIContentEditor) {
	h.aiContentEditor = editor
}

func (h *UserDashboardHandler) UseStreamLimiter(limiter *streamgate.Limiter) {
	h.streamLimiter = limiter
}

func (h *UserDashboardHandler) GetMyStats(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	stats, err := h.serviceFor(c).GetStats(&userID)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
	return c.JSON(http.StatusOK, stats)
}

func (h *UserDashboardHandler) GetExtensionSession(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	session, err := h.dashboardService.GetExtensionSession(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusUnauthorized, "unauthorized", "session user not found")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, session)
}

func (h *UserDashboardHandler) ListExtensionPrepublish(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	resp, err := h.dashboardService.ListExtensionPrepublish(userID)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) CreateExtensionHandoff(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	req := new(dto.CreateExtensionHandoffRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	handoff, err := h.dashboardService.CreateExtensionHandoff(userID, *req, extensionEventsCallbackURL(c))
	if err != nil {
		if errors.Is(err, services.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "project_id and supported platforms are required")
		}
		if errors.Is(err, services.ErrPublicationDisabled) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "publication is disabled for this project")
		}
		if errors.Is(err, services.ErrPublicationRequiresSync) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "sync prepublish draft before extension handoff")
		}
		if errors.Is(err, services.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, handoff)
}

func (h *UserDashboardHandler) RecordExtensionEvent(c echo.Context) error {
	req := new(dto.ExtensionEventCallbackRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	resp, err := h.dashboardService.RecordExtensionEvent(*req)
	if err != nil {
		if errors.Is(err, services.ErrExtensionCallbackTokenInvalid) ||
			errors.Is(err, services.ErrExtensionCallbackTokenExpired) {
			return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) ListMyProjects(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	page, limit := projectPaginationFromQuery(c)
	status := c.QueryParam("status")
	platform := c.QueryParam("platform")

	// Personal view: enforce scopeUserID, ignore any requested filterUserID
	resp, err := h.serviceFor(c).ListProjects(page, limit, status, "", platform, &userID)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
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

func (h *UserDashboardHandler) CreateProject(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	req := new(dto.CreateProjectRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	resp, err := h.serviceFor(c).CreateProject(userID, *req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "title, source_content and platforms are required")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusCreated, resp)
}

func (h *UserDashboardHandler) GetMyProject(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	idParam := c.Param("id")
	projectID, err := uuid.Parse(idParam)
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}

	project, err := h.serviceFor(c).GetProject(projectID, &userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, services.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, project)
}

func (h *UserDashboardHandler) UpdateProject(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	idParam := c.Param("id")
	projectID, err := uuid.Parse(idParam)
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}

	req := new(dto.UpdateProjectRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	project, err := h.serviceFor(c).UpdateProject(projectID, userID, *req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "title, source_content and platforms are required")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, services.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		if errors.Is(err, services.ErrProjectCollabUnavailable) {
			return sendError(c, http.StatusServiceUnavailable, "service_unavailable", "project collaboration unavailable")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, project)
}

func (h *UserDashboardHandler) SaveProjectContent(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	idParam := c.Param("id")
	projectID, err := uuid.Parse(idParam)
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}

	req := new(dto.SaveProjectContentRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	project, err := h.serviceFor(c).SaveProjectContent(projectID, userID, *req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "title and source_content are required")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, services.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		if errors.Is(err, services.ErrProjectCollabUnavailable) {
			return sendError(c, http.StatusServiceUnavailable, "service_unavailable", "project collaboration unavailable")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, project)
}

func (h *UserDashboardHandler) SaveProjectPlatforms(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	idParam := c.Param("id")
	projectID, err := uuid.Parse(idParam)
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}

	req := new(dto.SaveProjectPlatformsRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	project, err := h.serviceFor(c).SaveProjectPlatforms(projectID, userID, *req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "valid platforms are required")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, services.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, project)
}

func (h *UserDashboardHandler) CreateProjectMediaUpload(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}

	req := new(dto.CreateMediaUploadRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	resp, err := h.serviceFor(c).CreateProjectMediaUpload(projectID, userID, *req)
	if err != nil {
		return sendMediaAssetError(c, err)
	}
	return c.JSON(http.StatusCreated, resp)
}

func (h *UserDashboardHandler) CompleteMediaUpload(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	assetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid media asset UUID")
	}

	resp, err := h.serviceFor(c).CompleteMediaUpload(assetID, userID)
	if err != nil {
		return sendMediaAssetError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) ResolveMediaAssets(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	req := new(dto.ResolveMediaAssetsRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	resp, err := h.serviceFor(c).ResolveMediaAssets(userID, *req)
	if err != nil {
		return sendMediaAssetError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) ResolveMediaObjectRef(c echo.Context) error {
	req := new(dto.ResolveMediaObjectRefRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	resp, err := h.serviceFor(c).ResolveMediaObjectRef(*req)
	if err != nil {
		return sendMediaAssetError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) DeleteMediaAsset(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	assetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid media asset UUID")
	}

	if err := h.serviceFor(c).DeleteMediaAsset(assetID, userID); err != nil {
		return sendMediaAssetError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
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

func (h *UserDashboardHandler) CreateProjectCollabSession(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}

	session, err := h.serviceFor(c).CreateProjectCollabSession(projectID, userID)
	if err != nil {
		if errors.Is(err, services.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, services.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		if errors.Is(err, services.ErrProjectCollabUnavailable) {
			return sendError(c, http.StatusServiceUnavailable, "service_unavailable", "project collaboration unavailable")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, collabDocumentSessionResponse(session))
}

func (h *UserDashboardHandler) ListProjectCollaborators(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}

	resp, err := h.serviceFor(c).ListProjectCollaborators(projectID, userID)
	if err != nil {
		return sendProjectCollaboratorError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) AddProjectCollaborator(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}

	req := new(dto.AddProjectCollaboratorRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	collaborator, err := h.serviceFor(c).AddProjectCollaborator(projectID, userID, *req)
	if err != nil {
		return sendProjectCollaboratorError(c, err)
	}
	return c.JSON(http.StatusCreated, collaborator)
}

func (h *UserDashboardHandler) UpdateProjectCollaborator(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	targetUserID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid user UUID")
	}

	req := new(dto.UpdateProjectCollaboratorRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	collaborator, err := h.serviceFor(c).UpdateProjectCollaborator(projectID, userID, targetUserID, *req)
	if err != nil {
		return sendProjectCollaboratorError(c, err)
	}
	return c.JSON(http.StatusOK, collaborator)
}

func (h *UserDashboardHandler) RemoveProjectCollaborator(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	targetUserID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid user UUID")
	}

	if err := h.serviceFor(c).RemoveProjectCollaborator(projectID, userID, targetUserID); err != nil {
		return sendProjectCollaboratorError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
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

func (h *UserDashboardHandler) ListProjectActivities(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	resp, err := h.serviceFor(c).ListProjectActivities(projectID, userID, limit)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) ListProjectComments(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	resp, err := h.serviceFor(c).ListProjectComments(projectID, userID)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) CreateProjectComment(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	req := new(dto.CreateProjectCommentRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}
	comment, err := h.serviceFor(c).CreateProjectComment(projectID, userID, *req)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusCreated, comment)
}

func (h *UserDashboardHandler) UpdateProjectComment(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	commentID, err := uuid.Parse(c.Param("commentId"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid comment UUID")
	}
	req := new(dto.UpdateProjectCommentRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}
	comment, err := h.serviceFor(c).UpdateProjectComment(projectID, userID, commentID, *req)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, comment)
}

func (h *UserDashboardHandler) ListProjectVersions(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	resp, err := h.serviceFor(c).ListProjectVersions(projectID, userID)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) RestoreProjectVersion(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	versionID, err := uuid.Parse(c.Param("versionId"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid version UUID")
	}
	resp, err := h.serviceFor(c).RestoreProjectVersion(projectID, userID, versionID)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) ListProjectShareLinks(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	resp, err := h.serviceFor(c).ListProjectShareLinks(projectID, userID)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) CreateProjectShareLink(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	req := new(dto.CreateProjectShareLinkRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}
	link, err := h.serviceFor(c).CreateProjectShareLink(projectID, userID, *req, frontendBaseURL(c))
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusCreated, link)
}

func (h *UserDashboardHandler) AcceptProjectShareLink(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid share link token")
	}
	resp, err := h.serviceFor(c).AcceptProjectShareLink(token, userID)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) RevokeProjectShareLink(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	linkID, err := uuid.Parse(c.Param("linkId"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid share link UUID")
	}
	if err := h.serviceFor(c).RevokeProjectShareLink(projectID, userID, linkID); err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
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

func (h *UserDashboardHandler) ListWorkspaces(c echo.Context) error {
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

func (h *UserDashboardHandler) CreateWorkspace(c echo.Context) error {
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

func (h *UserDashboardHandler) ListWorkspaceProjects(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}

	page, limit := projectPaginationFromQuery(c)
	status := c.QueryParam("status")
	platform := c.QueryParam("platform")

	resp, err := h.serviceFor(c).ListWorkspaceProjects(workspaceID, userID, page, limit, status, platform)
	if err != nil {
		return sendWorkspaceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) CreateWorkspaceProject(c echo.Context) error {
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
		if errors.Is(err, services.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "title, source_content and platforms are required")
		}
		return sendWorkspaceError(c, err)
	}
	return c.JSON(http.StatusCreated, project)
}

func (h *UserDashboardHandler) GetWorkspace(c echo.Context) error {
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

func (h *UserDashboardHandler) UpdateWorkspace(c echo.Context) error {
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

func (h *UserDashboardHandler) ListWorkspaceMembers(c echo.Context) error {
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

func (h *UserDashboardHandler) ListWorkspaceActivities(c echo.Context) error {
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

func (h *UserDashboardHandler) AddWorkspaceMember(c echo.Context) error {
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

func (h *UserDashboardHandler) UpdateWorkspaceMember(c echo.Context) error {
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

func (h *UserDashboardHandler) RemoveWorkspaceMember(c echo.Context) error {
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

func sendWorkspaceError(c echo.Context, err error) error {
	if errors.Is(err, services.ErrInvalidWorkspace) || errors.Is(err, services.ErrInvalidWorkspaceMember) {
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
	includeContent := c.QueryParam("include_content") == "true"
	publications, err := h.serviceFor(c).GetProjectPublications(projectID, &userID, includeContent)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, services.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, publications)
}

func (h *UserDashboardHandler) SyncProjectPrepublish(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	idParam := c.Param("id")
	projectID, err := uuid.Parse(idParam)
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}

	req := new(dto.SyncPrepublishRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	publications, err := h.serviceFor(c).SyncProjectPrepublish(projectID, userID, *req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "at least one valid platform is required")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, services.ErrPublicationAlreadyPublishing) {
			return sendError(c, http.StatusConflict, "publish_in_progress", "publication is already publishing")
		}
		if errors.Is(err, services.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		if errors.Is(err, services.ErrProjectCollabUnavailable) {
			return sendError(c, http.StatusServiceUnavailable, "service_unavailable", "project collaboration unavailable")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, publications)
}

func (h *UserDashboardHandler) UpdateProjectPrepublishDraft(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}

	req := new(dto.UpdatePrepublishDraftRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	publications, err := h.serviceFor(c).UpdateProjectPrepublishDraft(projectID, userID, c.Param("platform"), *req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "valid platform and adapted_content are required")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project or publication not found")
		}
		if errors.Is(err, services.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, publications)
}

func (h *UserDashboardHandler) EditContentWithAI(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	if h.aiContentEditor == nil {
		return sendError(c, http.StatusServiceUnavailable, "ai_unavailable", services.ErrAIServiceUnavailable.Error())
	}

	req := new(dto.AIEditContentRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	lease, err := h.acquireAILease(c, userID, "content")
	if err != nil {
		if handled := streamgate.SendLimitError(c, err); handled != nil {
			return handled
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
	defer func() { _ = lease.Release(context.Background()) }()

	resp, err := h.aiContentEditor.EditContent(c.Request().Context(), *req)
	if err != nil {
		return sendAIEditError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) StreamEditContentWithAI(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	if h.aiContentEditor == nil {
		return sendError(c, http.StatusServiceUnavailable, "ai_unavailable", services.ErrAIServiceUnavailable.Error())
	}

	req := new(dto.AIEditContentRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	lease, err := h.acquireAILease(c, userID, "content-stream")
	if err != nil {
		if handled := streamgate.SendLimitError(c, err); handled != nil {
			return handled
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
	defer func() { _ = lease.Release(context.Background()) }()

	stream, err := h.aiContentEditor.StreamEditContent(c.Request().Context(), *req)
	if err != nil {
		return sendAIEditError(c, err)
	}
	return writeAIStream(c, stream, lease)
}

func (h *UserDashboardHandler) EditPrepublishWithAI(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	if h.aiContentEditor == nil {
		return sendError(c, http.StatusServiceUnavailable, "ai_unavailable", services.ErrAIServiceUnavailable.Error())
	}

	req := new(dto.AIEditPrepublishRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	lease, err := h.acquireAILease(c, userID, "prepublish")
	if err != nil {
		if handled := streamgate.SendLimitError(c, err); handled != nil {
			return handled
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
	defer func() { _ = lease.Release(context.Background()) }()

	resp, err := h.aiContentEditor.EditPrepublish(c.Request().Context(), *req)
	if err != nil {
		return sendAIEditError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) StreamEditPrepublishWithAI(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	if h.aiContentEditor == nil {
		return sendError(c, http.StatusServiceUnavailable, "ai_unavailable", services.ErrAIServiceUnavailable.Error())
	}

	req := new(dto.AIEditPrepublishRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	lease, err := h.acquireAILease(c, userID, "prepublish-stream")
	if err != nil {
		if handled := streamgate.SendLimitError(c, err); handled != nil {
			return handled
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
	defer func() { _ = lease.Release(context.Background()) }()

	stream, err := h.aiContentEditor.StreamEditPrepublish(c.Request().Context(), *req)
	if err != nil {
		return sendAIEditError(c, err)
	}
	return writeAIStream(c, stream, lease)
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

func (h *UserDashboardHandler) acquireAILease(c echo.Context, userID uuid.UUID, resource string) (*streamgate.Lease, error) {
	if h.streamLimiter == nil {
		return &streamgate.Lease{}, nil
	}
	tenantID, err := middleware.GetTenantIDFromContext(c)
	if err != nil {
		return nil, err
	}
	return h.streamLimiter.Acquire(c.Request().Context(), streamgate.AcquireRequest{
		Kind:     streamgate.KindAI,
		UserID:   userID,
		TenantID: tenantID,
		IP:       streamgate.ClientIP(c),
		Resource: resource,
	})
}

func writeAIStream(c echo.Context, stream *services.AIServiceStream, lease *streamgate.Lease) error {
	if stream == nil || stream.Body == nil {
		return sendError(c, http.StatusBadGateway, "ai_unavailable", services.ErrAIServiceUnavailable.Error())
	}
	defer func() { _ = stream.Body.Close() }()

	contentType := strings.TrimSpace(stream.ContentType)
	if contentType == "" {
		contentType = "text/markdown; charset=utf-8"
	}

	resp := c.Response()
	resp.Header().Set(echo.HeaderContentType, contentType)
	resp.Header().Set(echo.HeaderCacheControl, "no-cache")
	resp.Header().Set("X-Accel-Buffering", "no")
	if lease != nil && lease.ID != "" {
		resp.Header().Set("X-MPP-Stream-ID", lease.ID)
	}
	resp.WriteHeader(http.StatusOK)

	buffer := make([]byte, 1024)
	for {
		n, readErr := stream.Body.Read(buffer)
		if n > 0 {
			if _, err := resp.Write(buffer[:n]); err != nil {
				return err
			}
			resp.Flush()
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func (h *UserDashboardHandler) PublishProject(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	idParam := c.Param("id")
	projectID, err := uuid.Parse(idParam)
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}

	type PublishRequest struct {
		Platform       string   `json:"platform"`
		Platforms      []string `json:"platforms"`
		Mode           string   `json:"mode"`
		IdempotencyKey string   `json:"idempotency_key"`
	}
	req := new(PublishRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	if strings.EqualFold(strings.TrimSpace(req.Mode), "manual") {
		if len(req.Platforms) > 0 || !strings.EqualFold(strings.TrimSpace(req.Platform), "x") {
			return sendError(c, http.StatusBadRequest, "invalid_request", services.ErrManualPublishUnsupported.Error())
		}

		resp, err := h.serviceFor(c).CreateXPostIntent(projectID, &userID)
		if err != nil {
			if errors.Is(err, services.ErrPublicationDisabled) {
				return sendError(c, http.StatusBadRequest, "invalid_request", "publication is disabled for this project")
			}
			if errors.Is(err, services.ErrPublicationRequiresSync) {
				return sendError(c, http.StatusBadRequest, "invalid_request", "sync prepublish draft before publishing")
			}
			if errors.Is(err, services.ErrForbidden) {
				return sendError(c, http.StatusForbidden, "forbidden", err.Error())
			}
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}
		return c.JSON(http.StatusOK, resp)
	}

	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = strings.TrimSpace(c.Request().Header.Get("Idempotency-Key"))
	}
	if idempotencyKey == "" {
		idempotencyKey = uuid.New().String()
	}
	publishReq := services.PublishRequest{IdempotencyKey: idempotencyKey}

	if len(req.Platforms) > 0 {
		resp, err := h.serviceFor(c).BatchEnqueuePublishProject(c.Request().Context(), projectID, req.Platforms, &userID, publishReq)
		if err != nil {
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}
		return c.JSON(http.StatusOK, resp)
	}

	// Single platform fallback
	resp, err := h.serviceFor(c).EnqueuePublishProject(c.Request().Context(), projectID, req.Platform, &userID, publishReq)
	if err != nil {
		if errors.Is(err, services.ErrPublicationDisabled) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "publication is disabled for this project")
		}
		if errors.Is(err, services.ErrPublicationAlreadyPublishing) {
			return sendError(c, http.StatusConflict, "publish_in_progress", "publication is already publishing")
		}
		if errors.Is(err, services.ErrPublicationRequiresSync) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "sync prepublish draft before publishing")
		}
		if errors.Is(err, services.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) StartDouyinPublishSession(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}

	resp, err := h.serviceFor(c).StartDouyinPublishSession(c.Request().Context(), projectID, userID)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		if errors.Is(err, services.ErrPublicationDisabled) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "publication is disabled for this project")
		}
		if errors.Is(err, services.ErrPublicationRequiresSync) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "sync douyin prepublish draft before publishing")
		}
		if errors.Is(err, browsersession.ErrActiveSessionExists) {
			return sendError(c, http.StatusConflict, "conflict", err.Error())
		}
		if errors.Is(err, browsersession.ErrPlatformNotSupported) {
			return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"project_id":              projectID,
		"platform":                "douyin",
		"session_id":              resp.SessionID,
		"status":                  resp.Status,
		"stream_url":              resp.StreamURL,
		"stream_token_expires_at": resp.StreamTokenExpiresAt,
		"expires_at":              resp.ExpiresAt,
	})
}

func (h *UserDashboardHandler) GetWechatAccount(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	resp, err := h.serviceFor(c).GetWechatAccount(userID)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) SaveWechatAccount(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	req := new(dto.UpsertWechatAccountRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	resp, err := h.serviceFor(c).UpsertWechatAccount(userID, *req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidPlatformAccount) {
			return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) TestWechatAccount(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	req := new(dto.TestWechatAccountRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	resp, err := h.serviceFor(c).TestWechatAccount(userID, *req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidPlatformAccount) {
			return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) GetDouyinAccount(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	resp, err := h.serviceFor(c).GetDouyinAccount(userID)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) GetZhihuAccount(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	resp, err := h.serviceFor(c).GetZhihuAccount(userID)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) GetXAccount(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	resp, err := h.serviceFor(c).GetXAccount(userID)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) SaveXAccount(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	req := new(dto.UpsertXAccountRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	resp, err := h.serviceFor(c).UpsertXAccount(userID, *req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidPlatformAccount) {
			return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) TestXAccount(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	req := new(dto.TestXAccountRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	resp, err := h.serviceFor(c).TestXAccount(userID, *req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidPlatformAccount) {
			return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) StartXOAuth2(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	authURL, err := h.serviceFor(c).StartXOAuth2(userID, xOAuth2RedirectURI(c))
	if err != nil {
		if errors.Is(err, services.ErrXOAuth2NotConfigured) {
			return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
	return c.Redirect(http.StatusFound, authURL)
}

func (h *UserDashboardHandler) CompleteXOAuth2(c echo.Context) error {
	if c.QueryParam("error") != "" {
		return c.Redirect(http.StatusFound, xOAuth2SettingsRedirectURL("failed"))
	}

	_, err := h.serviceFor(c).CompleteXOAuth2(
		c.Request().Context(),
		c.QueryParam("state"),
		c.QueryParam("code"),
	)
	if err != nil {
		return c.Redirect(http.StatusFound, xOAuth2SettingsRedirectURL("failed"))
	}
	return c.Redirect(http.StatusFound, xOAuth2SettingsRedirectURL("connected"))
}

func xOAuth2RedirectURI(c echo.Context) string {
	if redirectURI := strings.TrimSpace(os.Getenv(xOAuth2RedirectURLEnv)); redirectURI != "" {
		return redirectURI
	}

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
	return proto + "://" + host + "/api/user/dashboard/settings/x/oauth2/callback"
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

func frontendBaseURL(c echo.Context) string {
	if baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(frontendBaseURLEnv)), "/"); baseURL != "" {
		return baseURL
	}

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
	return proto + "://" + host
}

func xOAuth2SettingsRedirectURL(status string) string {
	path := "/dashboard/settings?x_oauth=" + status
	if baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(frontendBaseURLEnv)), "/"); baseURL != "" {
		return baseURL + path
	}
	return path
}

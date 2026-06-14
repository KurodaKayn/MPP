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
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
	aisvc "github.com/kurodakayn/mpp-backend/internal/services/ai"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	dashboardsvc "github.com/kurodakayn/mpp-backend/internal/services/dashboard"
	extensionsvc "github.com/kurodakayn/mpp-backend/internal/services/extension"
	platformaccount "github.com/kurodakayn/mpp-backend/internal/services/platform_account"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
)

const (
	xOAuth2RedirectURLEnv = "X_OAUTH2_REDIRECT_URL"
	frontendBaseURLEnv    = "FRONTEND_BASE_URL"
)

type UserDashboardHandler struct {
	dashboardService *dashboardsvc.DashboardService
	aiContentEditor  aisvc.AIContentEditor
	streamLimiter    *streamgate.Limiter
}

func NewUserDashboardHandler(s *dashboardsvc.DashboardService) *UserDashboardHandler {
	return &UserDashboardHandler{dashboardService: s}
}

func (h *UserDashboardHandler) serviceFor(c echo.Context) *dashboardsvc.DashboardService {
	return h.dashboardService.WithContext(c.Request().Context())
}

func (h *UserDashboardHandler) UseAIContentEditor(editor aisvc.AIContentEditor) {
	h.aiContentEditor = editor
}

func (h *UserDashboardHandler) UseStreamLimiter(limiter *streamgate.Limiter) {
	h.streamLimiter = limiter
}

func (h *UserDashboardHandler) GetMyStats(c echo.Context) error {
	return h.withAuthenticatedDashboardRequest(c, func(dashReq *dashboardRequest) error {
		workspaceID, err := dashReq.optionalWorkspaceID()
		if err != nil {
			return err
		}
		var stats *dto.DashboardStatsResponse
		if workspaceID != uuid.Nil {
			stats, err = dashReq.service.GetWorkspaceStats(workspaceID, dashReq.userID)
		} else {
			stats, err = dashReq.service.GetStats(&dashReq.userID)
		}
		if err != nil {
			if errors.Is(err, accesspolicy.ErrForbidden) {
				return sendError(c, http.StatusForbidden, "forbidden", err.Error())
			}
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}
		return c.JSON(http.StatusOK, stats)
	})
}

func (h *UserDashboardHandler) GetExtensionSession(c echo.Context) error {
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

func (h *UserDashboardHandler) ListExtensionPrepublish(c echo.Context) error {
	return h.withAuthenticatedDashboardRequest(c, func(dashReq *dashboardRequest) error {
		resp, err := dashReq.service.ListExtensionPrepublish(dashReq.userID)
		if err != nil {
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusOK, resp)
	})
}

func (h *UserDashboardHandler) CreateExtensionHandoff(c echo.Context) error {
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

func (h *UserDashboardHandler) RecordExtensionEvent(c echo.Context) error {
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

func (h *UserDashboardHandler) ListMyProjects(c echo.Context) error {
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

func (h *UserDashboardHandler) CreateProject(c echo.Context) error {
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

func (h *UserDashboardHandler) ListContentTemplates(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	workspaceID, err := workspaceIDFromQuery(c)
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}
	resp, err := h.serviceFor(c).ListContentTemplates(userID, workspaceID)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) CreateContentTemplate(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	workspaceID, err := workspaceIDFromQuery(c)
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}
	req := new(dto.CreateContentTemplateRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}
	template, err := h.serviceFor(c).CreateContentTemplate(userID, workspaceID, *req)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusCreated, template)
}

func (h *UserDashboardHandler) ListBrandProfiles(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	workspaceID, err := workspaceIDFromQuery(c)
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}
	resp, err := h.serviceFor(c).ListBrandProfiles(userID, workspaceID)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) CreateBrandProfile(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	workspaceID, err := workspaceIDFromQuery(c)
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}
	req := new(dto.CreateBrandProfileRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}
	profile, err := h.serviceFor(c).CreateBrandProfile(userID, workspaceID, *req)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusCreated, profile)
}

func (h *UserDashboardHandler) ListWorkspaceContentTemplates(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}
	resp, err := h.serviceFor(c).ListContentTemplates(userID, workspaceID)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) CreateWorkspaceContentTemplate(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}
	req := new(dto.CreateContentTemplateRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}
	template, err := h.serviceFor(c).CreateContentTemplate(userID, workspaceID, *req)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusCreated, template)
}

func (h *UserDashboardHandler) ListWorkspaceBrandProfiles(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}
	resp, err := h.serviceFor(c).ListBrandProfiles(userID, workspaceID)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) CreateWorkspaceBrandProfile(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}
	req := new(dto.CreateBrandProfileRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}
	profile, err := h.serviceFor(c).CreateBrandProfile(userID, workspaceID, *req)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusCreated, profile)
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

func (h *UserDashboardHandler) DeleteProject(c echo.Context) error {
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

func (h *UserDashboardHandler) CreateProjectMediaUpload(c echo.Context) error {
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
		return sendWorkspaceError(c, err)
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

func (h *UserDashboardHandler) CreateProjectCollabSession(c echo.Context) error {
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
		return sendWorkspaceError(c, err)
	}

	session, err := h.serviceFor(c).CreateProjectCollabSession(projectID, userID)
	if err != nil {
		if errors.Is(err, projectsvc.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project")
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
	cursor := c.QueryParam("cursor")
	status := c.QueryParam("status")
	platform := c.QueryParam("platform")

	resp, err := h.serviceFor(c).ListWorkspaceProjectsCursor(workspaceID, userID, cursor, page, limit, status, platform)
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
		if errors.Is(err, projectsvc.ErrInvalidProject) {
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

func (h *UserDashboardHandler) ListWorkspaceInvites(c echo.Context) error {
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

func (h *UserDashboardHandler) CreateWorkspaceInvite(c echo.Context) error {
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

func (h *UserDashboardHandler) AcceptWorkspaceInvite(c echo.Context) error {
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

func (h *UserDashboardHandler) RevokeWorkspaceInvite(c echo.Context) error {
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
	if err := h.ensureProjectWorkspaceContext(c, projectID, userID); err != nil {
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendWorkspaceError(c, err)
	}

	// Personal view: enforce scopeUserID to check ownership
	includeContent := c.QueryParam("include_content") == "true"
	publications, err := h.serviceFor(c).GetProjectPublications(projectID, &userID, includeContent)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, publications)
}

func (h *UserDashboardHandler) ScheduleProjectPublication(c echo.Context) error {
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
		return sendWorkspaceError(c, err)
	}
	req := new(dto.SchedulePublicationRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}
	schedule, err := h.serviceFor(c).ScheduleProjectPublication(c.Request().Context(), projectID, userID, *req)
	if err != nil {
		return sendPublishScheduleError(c, err)
	}
	return c.JSON(http.StatusCreated, schedule)
}

func (h *UserDashboardHandler) CancelScheduledPublication(c echo.Context) error {
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
		return sendWorkspaceError(c, err)
	}
	scheduleID, err := uuid.Parse(c.Param("scheduleId"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid schedule UUID")
	}
	schedule, err := h.serviceFor(c).CancelScheduledPublication(c.Request().Context(), projectID, scheduleID, userID)
	if err != nil {
		return sendPublishScheduleError(c, err)
	}
	return c.JSON(http.StatusOK, schedule)
}

func (h *UserDashboardHandler) RetryScheduledPublication(c echo.Context) error {
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
		return sendWorkspaceError(c, err)
	}
	scheduleID, err := uuid.Parse(c.Param("scheduleId"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid schedule UUID")
	}
	schedule, err := h.serviceFor(c).RetryScheduledPublication(c.Request().Context(), projectID, scheduleID, userID)
	if err != nil {
		return sendPublishScheduleError(c, err)
	}
	return c.JSON(http.StatusOK, schedule)
}

func (h *UserDashboardHandler) ListWorkspacePublicationCalendar(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	workspaceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid workspace UUID")
	}
	from, err := parseDashboardTimeParam(c.QueryParam("from"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid from timestamp")
	}
	to, err := parseDashboardTimeParam(c.QueryParam("to"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid to timestamp")
	}
	resp, err := h.serviceFor(c).ListWorkspaceScheduledPublications(c.Request().Context(), workspaceID, userID, from, to)
	if err != nil {
		return sendPublishScheduleError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
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
	if err := h.ensureProjectWorkspaceContext(c, projectID, userID); err != nil {
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendWorkspaceError(c, err)
	}

	req := new(dto.SyncPrepublishRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	publications, err := h.serviceFor(c).SyncProjectPrepublish(projectID, userID, *req)
	if err != nil {
		if errors.Is(err, projectsvc.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "at least one valid platform is required")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, publishsvc.ErrPublicationAlreadyPublishing) {
			return sendError(c, http.StatusConflict, "publish_in_progress", "publication is already publishing")
		}
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		if errors.Is(err, projectsvc.ErrProjectCollabUnavailable) {
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
	if err := h.ensureProjectWorkspaceContext(c, projectID, userID); err != nil {
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendWorkspaceError(c, err)
	}

	req := new(dto.UpdatePrepublishDraftRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	publications, err := h.serviceFor(c).UpdateProjectPrepublishDraft(projectID, userID, c.Param("platform"), *req)
	if err != nil {
		if errors.Is(err, projectsvc.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "valid platform and adapted_content are required")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project or publication not found")
		}
		if errors.Is(err, accesspolicy.ErrForbidden) {
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
		return sendError(c, http.StatusServiceUnavailable, "ai_unavailable", aisvc.ErrAIServiceUnavailable.Error())
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
		return sendError(c, http.StatusServiceUnavailable, "ai_unavailable", aisvc.ErrAIServiceUnavailable.Error())
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
		return sendError(c, http.StatusServiceUnavailable, "ai_unavailable", aisvc.ErrAIServiceUnavailable.Error())
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
		return sendError(c, http.StatusServiceUnavailable, "ai_unavailable", aisvc.ErrAIServiceUnavailable.Error())
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

func writeAIStream(c echo.Context, stream *aisvc.AIServiceStream, lease *streamgate.Lease) error {
	if stream == nil || stream.Body == nil {
		return sendError(c, http.StatusBadGateway, "ai_unavailable", aisvc.ErrAIServiceUnavailable.Error())
	}
	defer func() { _ = stream.Body.Close() }()

	contentType := strings.TrimSpace(stream.ContentType)
	if contentType == "" {
		contentType = "text/markdown; charset=utf-8"
	}

	resp := c.Response()
	resp.Header().Set(echo.HeaderContentType, contentType)
	resp.Header().Set(echo.HeaderCacheControl, middleware.NoStoreCacheControl)
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
	if err := h.ensureProjectWorkspaceContext(c, projectID, userID); err != nil {
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendWorkspaceError(c, err)
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
			return sendError(c, http.StatusBadRequest, "invalid_request", publishsvc.ErrManualPublishUnsupported.Error())
		}

		resp, err := h.serviceFor(c).CreateXPostIntent(projectID, &userID)
		if err != nil {
			if errors.Is(err, publishsvc.ErrPublicationDisabled) {
				return sendError(c, http.StatusBadRequest, "invalid_request", "publication is disabled for this project")
			}
			if errors.Is(err, publishsvc.ErrPublicationRequiresSync) {
				return sendError(c, http.StatusBadRequest, "invalid_request", "sync prepublish draft before publishing")
			}
			if errors.Is(err, accesspolicy.ErrForbidden) {
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
	publishReq := publishsvc.PublishRequest{IdempotencyKey: idempotencyKey}

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
		if errors.Is(err, publishsvc.ErrPublicationDisabled) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "publication is disabled for this project")
		}
		if errors.Is(err, publishsvc.ErrPublicationAlreadyPublishing) {
			return sendError(c, http.StatusConflict, "publish_in_progress", "publication is already publishing")
		}
		if errors.Is(err, publishsvc.ErrPublicationRequiresSync) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "sync prepublish draft before publishing")
		}
		if errors.Is(err, accesspolicy.ErrForbidden) {
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
	if err := h.ensureProjectWorkspaceContext(c, projectID, userID); err != nil {
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendWorkspaceError(c, err)
	}

	resp, err := h.serviceFor(c).StartDouyinPublishSession(c.Request().Context(), projectID, userID)
	if err != nil {
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		if errors.Is(err, publishsvc.ErrPublicationDisabled) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "publication is disabled for this project")
		}
		if errors.Is(err, publishsvc.ErrPublicationRequiresSync) {
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
	return h.withWorkspaceAccountDashboardRequest(c, func(dashReq *dashboardRequest, workspaceID uuid.UUID) error {
		resp, err := dashReq.service.GetWorkspaceWechatAccount(dashReq.userID, workspaceID)
		if err != nil {
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusOK, resp)
	})
}

func (h *UserDashboardHandler) SaveWechatAccount(c echo.Context) error {
	return h.withWorkspaceAccountDashboardRequest(c, func(dashReq *dashboardRequest, workspaceID uuid.UUID) error {
		req := new(dto.UpsertWechatAccountRequest)
		if err := dashReq.bind(req); err != nil {
			return err
		}

		resp, err := dashReq.service.UpsertWorkspaceWechatAccount(dashReq.userID, workspaceID, *req)
		if err != nil {
			if errors.Is(err, platformaccount.ErrInvalidPlatformAccount) {
				return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
			}
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusOK, resp)
	})
}

func (h *UserDashboardHandler) TestWechatAccount(c echo.Context) error {
	return h.withWorkspaceAccountDashboardRequest(c, func(dashReq *dashboardRequest, workspaceID uuid.UUID) error {
		req := new(dto.TestWechatAccountRequest)
		if err := dashReq.bind(req); err != nil {
			return err
		}

		resp, err := dashReq.service.TestWorkspaceWechatAccount(dashReq.userID, workspaceID, *req)
		if err != nil {
			if errors.Is(err, platformaccount.ErrInvalidPlatformAccount) {
				return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
			}
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusOK, resp)
	})
}

func (h *UserDashboardHandler) GetDouyinAccount(c echo.Context) error {
	return h.withWorkspaceAccountDashboardRequest(c, func(dashReq *dashboardRequest, workspaceID uuid.UUID) error {
		resp, err := dashReq.service.GetWorkspaceDouyinAccount(dashReq.userID, workspaceID)
		if err != nil {
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusOK, resp)
	})
}

func (h *UserDashboardHandler) GetZhihuAccount(c echo.Context) error {
	return h.withWorkspaceAccountDashboardRequest(c, func(dashReq *dashboardRequest, workspaceID uuid.UUID) error {
		resp, err := dashReq.service.GetWorkspaceZhihuAccount(dashReq.userID, workspaceID)
		if err != nil {
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusOK, resp)
	})
}

func (h *UserDashboardHandler) GetXAccount(c echo.Context) error {
	return h.withWorkspaceAccountDashboardRequest(c, func(dashReq *dashboardRequest, workspaceID uuid.UUID) error {
		resp, err := dashReq.service.GetWorkspaceXAccount(dashReq.userID, workspaceID)
		if err != nil {
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusOK, resp)
	})
}

func (h *UserDashboardHandler) SaveXAccount(c echo.Context) error {
	return h.withWorkspaceAccountDashboardRequest(c, func(dashReq *dashboardRequest, workspaceID uuid.UUID) error {
		req := new(dto.UpsertXAccountRequest)
		if err := dashReq.bind(req); err != nil {
			return err
		}

		resp, err := dashReq.service.UpsertWorkspaceXAccount(dashReq.userID, workspaceID, *req)
		if err != nil {
			if errors.Is(err, platformaccount.ErrInvalidPlatformAccount) {
				return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
			}
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusOK, resp)
	})
}

func (h *UserDashboardHandler) TestXAccount(c echo.Context) error {
	return h.withWorkspaceAccountDashboardRequest(c, func(dashReq *dashboardRequest, workspaceID uuid.UUID) error {
		req := new(dto.TestXAccountRequest)
		if err := dashReq.bind(req); err != nil {
			return err
		}

		resp, err := dashReq.service.TestWorkspaceXAccount(dashReq.userID, workspaceID, *req)
		if err != nil {
			if errors.Is(err, platformaccount.ErrInvalidPlatformAccount) {
				return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
			}
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusOK, resp)
	})
}

func (h *UserDashboardHandler) StartXOAuth2(c echo.Context) error {
	return h.withWorkspaceAccountDashboardRequest(c, func(dashReq *dashboardRequest, workspaceID uuid.UUID) error {
		authURL, err := dashReq.service.StartWorkspaceXOAuth2(dashReq.userID, workspaceID, xOAuth2RedirectURI(c))
		if err != nil {
			if errors.Is(err, platformaccount.ErrXOAuth2NotConfigured) {
				return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
			}
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.Redirect(http.StatusFound, authURL)
	})
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

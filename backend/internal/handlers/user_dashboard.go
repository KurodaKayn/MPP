package handlers

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/services"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

const (
	xOAuth2RedirectURLEnv = "X_OAUTH2_REDIRECT_URL"
	frontendBaseURLEnv    = "FRONTEND_BASE_URL"
)

type UserDashboardHandler struct {
	dashboardService *services.DashboardService
	aiContentEditor  services.AIContentEditor
}

func NewUserDashboardHandler(s *services.DashboardService) *UserDashboardHandler {
	return &UserDashboardHandler{dashboardService: s}
}

func (h *UserDashboardHandler) UseAIContentEditor(editor services.AIContentEditor) {
	h.aiContentEditor = editor
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

func (h *UserDashboardHandler) CreateProject(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	req := new(dto.CreateProjectRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	resp, err := h.dashboardService.CreateProject(userID, *req)
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

	project, err := h.dashboardService.GetProject(projectID, &userID)
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

	project, err := h.dashboardService.UpdateProject(projectID, userID, *req)
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

	project, err := h.dashboardService.SaveProjectContent(projectID, userID, *req)
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

	project, err := h.dashboardService.SaveProjectPlatforms(projectID, userID, *req)
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
	publications, err := h.dashboardService.GetProjectPublications(projectID, &userID, includeContent)
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

	publications, err := h.dashboardService.SyncProjectPrepublish(projectID, userID, *req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "at least one valid platform is required")
		}
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

	publications, err := h.dashboardService.UpdateProjectPrepublishDraft(projectID, userID, c.Param("platform"), *req)
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
	if _, err := middleware.GetUserIDFromContext(c); err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	if h.aiContentEditor == nil {
		return sendError(c, http.StatusServiceUnavailable, "ai_unavailable", services.ErrAIServiceUnavailable.Error())
	}

	req := new(dto.AIEditContentRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	resp, err := h.aiContentEditor.EditContent(c.Request().Context(), *req)
	if err != nil {
		return sendAIEditError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) StreamEditContentWithAI(c echo.Context) error {
	if _, err := middleware.GetUserIDFromContext(c); err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	if h.aiContentEditor == nil {
		return sendError(c, http.StatusServiceUnavailable, "ai_unavailable", services.ErrAIServiceUnavailable.Error())
	}

	req := new(dto.AIEditContentRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	stream, err := h.aiContentEditor.StreamEditContent(c.Request().Context(), *req)
	if err != nil {
		return sendAIEditError(c, err)
	}
	return writeAIStream(c, stream)
}

func (h *UserDashboardHandler) EditPrepublishWithAI(c echo.Context) error {
	if _, err := middleware.GetUserIDFromContext(c); err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	if h.aiContentEditor == nil {
		return sendError(c, http.StatusServiceUnavailable, "ai_unavailable", services.ErrAIServiceUnavailable.Error())
	}

	req := new(dto.AIEditPrepublishRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	resp, err := h.aiContentEditor.EditPrepublish(c.Request().Context(), *req)
	if err != nil {
		return sendAIEditError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserDashboardHandler) StreamEditPrepublishWithAI(c echo.Context) error {
	if _, err := middleware.GetUserIDFromContext(c); err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	if h.aiContentEditor == nil {
		return sendError(c, http.StatusServiceUnavailable, "ai_unavailable", services.ErrAIServiceUnavailable.Error())
	}

	req := new(dto.AIEditPrepublishRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	stream, err := h.aiContentEditor.StreamEditPrepublish(c.Request().Context(), *req)
	if err != nil {
		return sendAIEditError(c, err)
	}
	return writeAIStream(c, stream)
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

func writeAIStream(c echo.Context, stream *services.AIServiceStream) error {
	if stream == nil || stream.Body == nil {
		return sendError(c, http.StatusBadGateway, "ai_unavailable", services.ErrAIServiceUnavailable.Error())
	}
	defer stream.Body.Close()

	contentType := strings.TrimSpace(stream.ContentType)
	if contentType == "" {
		contentType = "text/markdown; charset=utf-8"
	}

	resp := c.Response()
	resp.Header().Set(echo.HeaderContentType, contentType)
	resp.Header().Set(echo.HeaderCacheControl, "no-cache")
	resp.Header().Set("X-Accel-Buffering", "no")
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
		Platform  string   `json:"platform"`
		Platforms []string `json:"platforms"`
		Mode      string   `json:"mode"`
	}
	req := new(PublishRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	if strings.EqualFold(strings.TrimSpace(req.Mode), "manual") {
		if len(req.Platforms) > 0 || !strings.EqualFold(strings.TrimSpace(req.Platform), "x") {
			return sendError(c, http.StatusBadRequest, "invalid_request", services.ErrManualPublishUnsupported.Error())
		}

		resp, err := h.dashboardService.CreateXPostIntent(projectID, &userID)
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

	if len(req.Platforms) > 0 {
		resp, err := h.dashboardService.BatchEnqueuePublishProject(c.Request().Context(), projectID, req.Platforms, &userID)
		if err != nil {
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}
		return c.JSON(http.StatusOK, resp)
	}

	// Single platform fallback
	resp, err := h.dashboardService.EnqueuePublishProject(c.Request().Context(), projectID, req.Platform, &userID)
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

	resp, err := h.dashboardService.StartDouyinPublishSession(c.Request().Context(), projectID, userID)
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

	return c.JSON(http.StatusCreated, map[string]interface{}{
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

	resp, err := h.dashboardService.GetWechatAccount(userID)
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

	resp, err := h.dashboardService.UpsertWechatAccount(userID, *req)
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

	resp, err := h.dashboardService.TestWechatAccount(userID, *req)
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

	resp, err := h.dashboardService.GetDouyinAccount(userID)
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

	resp, err := h.dashboardService.GetZhihuAccount(userID)
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

	resp, err := h.dashboardService.GetXAccount(userID)
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

	resp, err := h.dashboardService.UpsertXAccount(userID, *req)
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

	resp, err := h.dashboardService.TestXAccount(userID, *req)
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

	authURL, err := h.dashboardService.StartXOAuth2(userID, xOAuth2RedirectURI(c))
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

	_, err := h.dashboardService.CompleteXOAuth2(
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

func xOAuth2SettingsRedirectURL(status string) string {
	path := "/dashboard/settings?x_oauth=" + status
	if baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(frontendBaseURLEnv)), "/"); baseURL != "" {
		return baseURL + path
	}
	return path
}

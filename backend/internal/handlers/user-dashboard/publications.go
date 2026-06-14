package userdashboard

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
)

func (h *Handler) GetMyProjectPublications(c echo.Context) error {
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

func (h *Handler) ScheduleProjectPublication(c echo.Context) error {
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

func (h *Handler) CancelScheduledPublication(c echo.Context) error {
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

func (h *Handler) RetryScheduledPublication(c echo.Context) error {
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

func (h *Handler) ListWorkspacePublicationCalendar(c echo.Context) error {
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

func (h *Handler) SyncProjectPrepublish(c echo.Context) error {
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

func (h *Handler) UpdateProjectPrepublishDraft(c echo.Context) error {
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

func (h *Handler) PublishProject(c echo.Context) error {
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

func (h *Handler) StartDouyinPublishSession(c echo.Context) error {
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

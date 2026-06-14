package userdashboard

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/pkg/streamgate"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
	aisvc "github.com/kurodakayn/mpp-backend/internal/services/ai"
)

func (h *Handler) EditContentWithAI(c echo.Context) error {
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

	// Quota gate: check before calling AI service when workspace_id is provided.
	workspaceID, _ := workspaceIDFromQuery(c)
	if workspaceID != uuid.Nil && h.quotaSvc != nil {
		if err := h.quotaSvc.CheckQuota(c.Request().Context(), workspaceID); err != nil {
			return sendError(c, http.StatusTooManyRequests, "quota_exceeded", err.Error())
		}
	}

	resp, err := h.aiContentEditor.EditContent(c.Request().Context(), *req)
	if err != nil {
		return sendAIEditError(c, err)
	}

	// Record real usage best-effort after successful call.
	if workspaceID != uuid.Nil && h.quotaSvc != nil && resp.Usage != nil {
		if recErr := h.quotaSvc.RecordUsage(c.Request().Context(), workspaceID, userID, nil, "edit_content", resp.Usage); recErr != nil {
			log.Printf("[quota] RecordUsage failed workspace=%s kind=edit_content: %v", workspaceID, recErr)
		}
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) StreamEditContentWithAI(c echo.Context) error {
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

func (h *Handler) EditPrepublishWithAI(c echo.Context) error {
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

	// Quota gate: check before calling AI service when workspace_id is provided.
	workspaceID, _ := workspaceIDFromQuery(c)
	if workspaceID != uuid.Nil && h.quotaSvc != nil {
		if err := h.quotaSvc.CheckQuota(c.Request().Context(), workspaceID); err != nil {
			return sendError(c, http.StatusTooManyRequests, "quota_exceeded", err.Error())
		}
	}

	resp, err := h.aiContentEditor.EditPrepublish(c.Request().Context(), *req)
	if err != nil {
		return sendAIEditError(c, err)
	}

	// Record real usage best-effort after successful call.
	if workspaceID != uuid.Nil && h.quotaSvc != nil && resp.Usage != nil {
		if recErr := h.quotaSvc.RecordUsage(c.Request().Context(), workspaceID, userID, nil, "edit_prepublish", resp.Usage); recErr != nil {
			log.Printf("[quota] RecordUsage failed workspace=%s kind=edit_prepublish: %v", workspaceID, recErr)
		}
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) StreamEditPrepublishWithAI(c echo.Context) error {
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

func (h *Handler) StartAIDraftingSession(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	if h.aiDrafting == nil {
		return sendError(c, http.StatusServiceUnavailable, "ai_unavailable", aisvc.ErrAIServiceUnavailable.Error())
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
		return sendWorkspaceError(c, err)
	}
	if _, err := h.serviceFor(c).GetProject(projectID, &userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	var req struct {
		Message string `json:"message"`
		Title   string `json:"title"`
	}
	if err := c.Bind(&req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}
	if strings.TrimSpace(req.Message) == "" {
		return sendError(c, http.StatusBadRequest, "invalid_request", "message is required")
	}

	lease, err := h.acquireAILease(c, userID, "drafting")
	if err != nil {
		if handled := streamgate.SendLimitError(c, err); handled != nil {
			return handled
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
	defer func() { _ = lease.Release(context.Background()) }()

	stream := make(chan string, 32)
	errCh := make(chan error, 1)
	go func() {
		defer close(stream)
		_, runErr := h.aiDrafting.Start(c.Request().Context(), aisvc.StartDraftingSessionRequest{
			ProjectID: projectID,
			UserID:    userID,
			Message:   req.Message,
			Title:     req.Title,
		}, stream)
		errCh <- runErr
	}()

	if err := writeAIDraftingEventStream(c, stream, lease); err != nil {
		return err
	}
	if runErr := <-errCh; runErr != nil {
		return nil
	}
	return nil
}

func (h *Handler) ContinueAIDraftingSession(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	if h.aiDrafting == nil {
		return sendError(c, http.StatusServiceUnavailable, "ai_unavailable", aisvc.ErrAIServiceUnavailable.Error())
	}
	sessionID, err := uuid.Parse(c.Param("sessionId"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid session UUID")
	}
	session, err := h.aiDrafting.GetSession(c.Request().Context(), sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "session not found")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
	if _, err := h.serviceFor(c).GetProject(session.ProjectID, &userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := c.Bind(&req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}
	if strings.TrimSpace(req.Message) == "" {
		return sendError(c, http.StatusBadRequest, "invalid_request", "message is required")
	}

	lease, err := h.acquireAILease(c, userID, "drafting")
	if err != nil {
		if handled := streamgate.SendLimitError(c, err); handled != nil {
			return handled
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
	defer func() { _ = lease.Release(context.Background()) }()

	stream := make(chan string, 32)
	errCh := make(chan error, 1)
	go func() {
		defer close(stream)
		_, runErr := h.aiDrafting.Continue(c.Request().Context(), aisvc.ContinueDraftingSessionRequest{
			SessionID: sessionID,
			UserID:    userID,
			Message:   req.Message,
		}, stream)
		errCh <- runErr
	}()
	if err := writeAIDraftingEventStream(c, stream, lease); err != nil {
		return err
	}
	if runErr := <-errCh; runErr != nil {
		return nil
	}
	return nil
}

func (h *Handler) ReplayAIDraftingSession(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	if h.aiDrafting == nil {
		return sendError(c, http.StatusServiceUnavailable, "ai_unavailable", aisvc.ErrAIServiceUnavailable.Error())
	}
	sessionID, err := uuid.Parse(c.Param("sessionId"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid session UUID")
	}
	session, err := h.aiDrafting.GetSession(c.Request().Context(), sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "session not found")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
	if _, err := h.serviceFor(c).GetProject(session.ProjectID, &userID); err != nil {
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
	replay, err := h.aiDrafting.Replay(c.Request().Context(), sessionID)
	if err != nil {
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
	return c.JSON(http.StatusOK, replay)
}

func writeAIDraftingEventStream(c echo.Context, stream <-chan string, lease *streamgate.Lease) error {
	resp := c.Response()
	resp.Header().Set(echo.HeaderContentType, "text/event-stream; charset=utf-8")
	resp.Header().Set(echo.HeaderCacheControl, middleware.NoStoreCacheControl)
	resp.Header().Set("X-Accel-Buffering", "no")
	if lease != nil && lease.ID != "" {
		resp.Header().Set("X-MPP-Stream-ID", lease.ID)
	}
	resp.WriteHeader(http.StatusOK)
	for chunk := range stream {
		if _, err := resp.Write([]byte(chunk)); err != nil {
			return err
		}
		resp.Flush()
	}
	return nil
}

func (h *Handler) acquireAILease(c echo.Context, userID uuid.UUID, resource string) (*streamgate.Lease, error) {
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

package userdashboard

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/pkg/streamgate"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
	aisvc "github.com/kurodakayn/mpp-backend/internal/services/ai"
	dashboardsvc "github.com/kurodakayn/mpp-backend/internal/services/dashboard"
)

const (
	xOAuth2RedirectURLEnv = "X_OAUTH2_REDIRECT_URL"
	frontendBaseURLEnv    = "FRONTEND_BASE_URL"
)

type Handler struct {
	dashboardService *dashboardsvc.DashboardService
	aiContentEditor  aisvc.AIContentEditor
	aiDrafting       *aisvc.DraftingService
	quotaSvc         *aisvc.QuotaService
	streamLimiter    *streamgate.Limiter
}

func New(s *dashboardsvc.DashboardService) *Handler {
	return &Handler{dashboardService: s}
}

func (h *Handler) serviceFor(c echo.Context) *dashboardsvc.DashboardService {
	return h.dashboardService.WithContext(c.Request().Context())
}

func (h *Handler) UseAIContentEditor(editor aisvc.AIContentEditor) {
	h.aiContentEditor = editor
}

func (h *Handler) UseAIDraftingService(svc *aisvc.DraftingService) {
	h.aiDrafting = svc
}

func (h *Handler) UseQuotaService(svc *aisvc.QuotaService) {
	h.quotaSvc = svc
}

func (h *Handler) UseStreamLimiter(limiter *streamgate.Limiter) {
	h.streamLimiter = limiter
}

func (h *Handler) GetMyStats(c echo.Context) error {
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

package userdashboard

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	platformaccount "github.com/kurodakayn/mpp-backend/internal/services/platform_account"
)

func (h *Handler) GetWechatAccount(c echo.Context) error {
	return h.withWorkspaceAccountDashboardRequest(c, func(dashReq *dashboardRequest, workspaceID uuid.UUID) error {
		resp, err := dashReq.service.GetWorkspaceWechatAccount(dashReq.userID, workspaceID)
		if err != nil {
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusOK, resp)
	})
}

func (h *Handler) SaveWechatAccount(c echo.Context) error {
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

func (h *Handler) TestWechatAccount(c echo.Context) error {
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

func (h *Handler) GetDouyinAccount(c echo.Context) error {
	return h.withWorkspaceAccountDashboardRequest(c, func(dashReq *dashboardRequest, workspaceID uuid.UUID) error {
		resp, err := dashReq.service.GetWorkspaceDouyinAccount(dashReq.userID, workspaceID)
		if err != nil {
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusOK, resp)
	})
}

func (h *Handler) GetZhihuAccount(c echo.Context) error {
	return h.withWorkspaceAccountDashboardRequest(c, func(dashReq *dashboardRequest, workspaceID uuid.UUID) error {
		resp, err := dashReq.service.GetWorkspaceZhihuAccount(dashReq.userID, workspaceID)
		if err != nil {
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusOK, resp)
	})
}

func (h *Handler) GetXAccount(c echo.Context) error {
	return h.withWorkspaceAccountDashboardRequest(c, func(dashReq *dashboardRequest, workspaceID uuid.UUID) error {
		resp, err := dashReq.service.GetWorkspaceXAccount(dashReq.userID, workspaceID)
		if err != nil {
			return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}

		return c.JSON(http.StatusOK, resp)
	})
}

func (h *Handler) SaveXAccount(c echo.Context) error {
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

func (h *Handler) TestXAccount(c echo.Context) error {
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

func (h *Handler) StartXOAuth2(c echo.Context) error {
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

func (h *Handler) CompleteXOAuth2(c echo.Context) error {
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

func xOAuth2SettingsRedirectURL(status string) string {
	path := "/dashboard/settings?x_oauth=" + status
	if baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(frontendBaseURLEnv)), "/"); baseURL != "" {
		return baseURL + path
	}
	return path
}

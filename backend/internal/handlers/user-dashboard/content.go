package userdashboard

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
)

func (h *Handler) ListContentTemplates(c echo.Context) error {
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

func (h *Handler) CreateContentTemplate(c echo.Context) error {
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

func (h *Handler) ListBrandProfiles(c echo.Context) error {
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

func (h *Handler) CreateBrandProfile(c echo.Context) error {
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

func (h *Handler) ListWorkspaceContentTemplates(c echo.Context) error {
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

func (h *Handler) CreateWorkspaceContentTemplate(c echo.Context) error {
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

func (h *Handler) ListWorkspaceBrandProfiles(c echo.Context) error {
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

func (h *Handler) CreateWorkspaceBrandProfile(c echo.Context) error {
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

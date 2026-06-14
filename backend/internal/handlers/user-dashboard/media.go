package userdashboard

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
)

func (h *Handler) CreateProjectMediaUpload(c echo.Context) error {
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

func (h *Handler) CompleteMediaUpload(c echo.Context) error {
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

func (h *Handler) ResolveMediaAssets(c echo.Context) error {
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

func (h *Handler) ResolveMediaObjectRef(c echo.Context) error {
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

func (h *Handler) DeleteMediaAsset(c echo.Context) error {
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

package handlers

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/labstack/echo/v4"
)

type BrowserSessionHandler struct {
	service *services.BrowserSessionService
}

func NewBrowserSessionHandler(service *services.BrowserSessionService) *BrowserSessionHandler {
	return &BrowserSessionHandler{service: service}
}

func (h *BrowserSessionHandler) StartSession(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	platform := c.Param("platform")

	resp, err := h.service.StartSession(c.Request().Context(), userID, platform)
	if err != nil {
		if err == services.ErrActiveSessionExists {
			return sendError(c, http.StatusConflict, "conflict", err.Error())
		}
		if err == services.ErrPlatformNotSupported {
			return sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusCreated, resp)
}

func (h *BrowserSessionHandler) GetSession(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid session id")
	}

	resp, err := h.service.GetSession(c.Request().Context(), userID, id)
	if err != nil {
		if err == services.ErrSessionNotFound {
			return sendError(c, http.StatusNotFound, "not_found", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *BrowserSessionHandler) CompleteSession(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid session id")
	}

	resp, err := h.service.CompleteSession(c.Request().Context(), userID, id)
	if err != nil {
		if err == services.ErrSessionNotFound {
			return sendError(c, http.StatusNotFound, "not_found", err.Error())
		}
		// Assuming 422 for login not detected yet as per design
		return sendError(c, http.StatusUnprocessableEntity, "unprocessable_entity", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *BrowserSessionHandler) CancelSession(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid session id")
	}

	if err := h.service.CancelSession(c.Request().Context(), userID, id); err != nil {
		if err == services.ErrSessionNotFound {
			return sendError(c, http.StatusNotFound, "not_found", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

package handlers

import (
	"errors"
	"net/http"

	"github.com/kurodakayn/mpp-backend/internal/contracts"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/labstack/echo/v4"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

type CollabDocumentHandler struct {
	service *services.CollabDocumentService
}

func NewCollabDocumentHandler(service *services.CollabDocumentService) *CollabDocumentHandler {
	return &CollabDocumentHandler{service: service}
}

func (h *CollabDocumentHandler) serviceFor(c echo.Context) *services.CollabDocumentService {
	return h.service.WithContext(c.Request().Context())
}

func (h *CollabDocumentHandler) CreateDocument(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	req := new(contracts.CreateCollabDocumentRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	document, err := h.serviceFor(c).CreateDocument(c.Request().Context(), userID, req.Title)
	if err != nil {
		if errors.Is(err, services.ErrInvalidCollabDocument) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "title is required")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusCreated, collabDocumentResponse(document))
}

func collabDocumentResponse(document *models.CollabDocument) contracts.CollabDocument {
	var lastEditedBy *openapi_types.UUID
	if document.LastEditedBy != nil {
		value := openapi_types.UUID(*document.LastEditedBy)
		lastEditedBy = &value
	}

	return contracts.CollabDocument{
		CreatedAt:     document.CreatedAt,
		CurrentSeq:    document.CurrentSeq,
		Id:            openapi_types.UUID(document.ID),
		LastEditedAt:  document.LastEditedAt,
		LastEditedBy:  lastEditedBy,
		OwnerUserId:   openapi_types.UUID(document.OwnerUserID),
		SchemaVersion: document.SchemaVersion,
		Status:        contracts.CollabDocumentStatus(document.Status),
		Title:         document.Title,
		UpdatedAt:     document.UpdatedAt,
	}
}

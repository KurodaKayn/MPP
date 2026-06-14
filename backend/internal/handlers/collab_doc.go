package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/contracts"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/models"
	collabdoc "github.com/kurodakayn/mpp-backend/internal/services/collabdoc"
)

type CollabDocumentHandler struct {
	service *collabdoc.Service
}

func NewCollabDocumentHandler(service *collabdoc.Service) *CollabDocumentHandler {
	return &CollabDocumentHandler{service: service}
}

func (h *CollabDocumentHandler) serviceFor(c echo.Context) *collabdoc.Service {
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
		if errors.Is(err, collabdoc.ErrInvalidDocument) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "title is required")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusCreated, collabDocumentResponse(document))
}

func (h *CollabDocumentHandler) ListDocuments(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	result, err := h.serviceFor(c).ListDocuments(
		c.Request().Context(),
		userID,
		intQueryParam(c, "page"),
		intQueryParam(c, "limit"),
	)
	if err != nil {
		if errors.Is(err, collabdoc.ErrInvalidDocument) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "invalid user")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, contracts.PaginationCollabDocuments{
		Items:      collabDocumentResponses(result.Items),
		Page:       result.Page,
		Limit:      result.Limit,
		Total:      int(result.Total),
		TotalPages: result.TotalPages,
	})
}

func (h *CollabDocumentHandler) GetDocument(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	documentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid document UUID")
	}

	document, err := h.serviceFor(c).GetDocument(c.Request().Context(), userID, documentID)
	if err != nil {
		if errors.Is(err, collabdoc.ErrInvalidDocument) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "invalid document")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "document not found")
		}
		if errors.Is(err, collabdoc.ErrDocumentForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, collabDocumentResponse(document))
}

func (h *CollabDocumentHandler) UpdateDocument(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	documentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid document UUID")
	}

	req := new(contracts.UpdateCollabDocumentRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	document, err := h.serviceFor(c).UpdateDocumentTitle(c.Request().Context(), userID, documentID, req.Title)
	if err != nil {
		if errors.Is(err, collabdoc.ErrInvalidDocument) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "title is required")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "document not found")
		}
		if errors.Is(err, collabdoc.ErrDocumentForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, collabDocumentResponse(document))
}

func (h *CollabDocumentHandler) CreateSession(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	documentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid document UUID")
	}

	session, err := h.serviceFor(c).CreateSession(c.Request().Context(), userID, documentID)
	if err != nil {
		if errors.Is(err, collabdoc.ErrInvalidDocument) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "invalid session request")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "document not found")
		}
		if errors.Is(err, collabdoc.ErrDocumentForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, collabDocumentSessionResponse(session))
}

func collabDocumentSessionResponse(session *collabdoc.Session) contracts.CollabDocumentSession {
	return contracts.CollabDocumentSession{
		DocumentId:   openapi_types.UUID(session.DocumentID),
		Role:         contracts.CollabDocumentRole(session.Role),
		WebsocketUrl: session.WebsocketURL,
		Token:        session.Token,
		ExpiresAt:    session.ExpiresAt,
		Limits: contracts.CollabSessionLimits{
			MaxMessageBytes:  session.Limits.MaxMessageBytes,
			HeartbeatSeconds: session.Limits.HeartbeatSeconds,
		},
	}
}

func intQueryParam(c echo.Context, name string) int {
	value, _ := strconv.Atoi(c.QueryParam(name))
	return value
}

func collabDocumentResponses(documents []models.CollabDocument) []contracts.CollabDocument {
	items := make([]contracts.CollabDocument, 0, len(documents))
	for i := range documents {
		items = append(items, collabDocumentResponse(&documents[i]))
	}
	return items
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

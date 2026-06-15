package userdashboard

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/contracts"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/middleware"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
	collabdoc "github.com/kurodakayn/mpp-backend/internal/services/collabdoc"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
)

func (h *Handler) CreateProjectCollabSession(c echo.Context) error {
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

	session, err := h.serviceFor(c).CreateProjectCollabSession(projectID, userID)
	if err != nil {
		if errors.Is(err, projectsvc.ErrInvalidProject) {
			return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sendError(c, http.StatusNotFound, "not_found", "project not found")
		}
		if errors.Is(err, accesspolicy.ErrForbidden) {
			return sendError(c, http.StatusForbidden, "forbidden", err.Error())
		}
		if errors.Is(err, projectsvc.ErrProjectCollabUnavailable) {
			return sendError(c, http.StatusServiceUnavailable, "service_unavailable", "project collaboration unavailable")
		}
		return sendError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}

	return c.JSON(http.StatusOK, collabDocumentSessionResponse(session))
}

func (h *Handler) ListProjectCollaborators(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}

	resp, err := h.serviceFor(c).ListProjectCollaborators(projectID, userID)
	if err != nil {
		return sendProjectCollaboratorError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) ListOwnedProjectCollaboratorSummaries(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	resp, err := h.serviceFor(c).ListOwnedProjectCollaboratorSummaries(userID)
	if err != nil {
		return sendProjectCollaboratorError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) AddProjectCollaborator(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}

	req := new(dto.AddProjectCollaboratorRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	collaborator, err := h.serviceFor(c).AddProjectCollaborator(projectID, userID, *req)
	if err != nil {
		return sendProjectCollaboratorError(c, err)
	}
	return c.JSON(http.StatusCreated, collaborator)
}

func (h *Handler) UpdateProjectCollaborator(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	targetUserID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid user UUID")
	}

	req := new(dto.UpdateProjectCollaboratorRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}

	collaborator, err := h.serviceFor(c).UpdateProjectCollaborator(projectID, userID, targetUserID, *req)
	if err != nil {
		return sendProjectCollaboratorError(c, err)
	}
	return c.JSON(http.StatusOK, collaborator)
}

func (h *Handler) RemoveProjectCollaborator(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	targetUserID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid user UUID")
	}

	if err := h.serviceFor(c).RemoveProjectCollaborator(projectID, userID, targetUserID); err != nil {
		return sendProjectCollaboratorError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) ListProjectActivities(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	resp, err := h.serviceFor(c).ListProjectActivities(projectID, userID, limit)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) ListProjectComments(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	resp, err := h.serviceFor(c).ListProjectComments(projectID, userID)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) CreateProjectComment(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	req := new(dto.CreateProjectCommentRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}
	comment, err := h.serviceFor(c).CreateProjectComment(projectID, userID, *req)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusCreated, comment)
}

func (h *Handler) UpdateProjectComment(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	commentID, err := uuid.Parse(c.Param("commentId"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid comment UUID")
	}
	req := new(dto.UpdateProjectCommentRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}
	comment, err := h.serviceFor(c).UpdateProjectComment(projectID, userID, commentID, *req)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, comment)
}

func (h *Handler) ListProjectVersions(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	resp, err := h.serviceFor(c).ListProjectVersions(projectID, userID)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) RestoreProjectVersion(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	versionID, err := uuid.Parse(c.Param("versionId"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid version UUID")
	}
	resp, err := h.serviceFor(c).RestoreProjectVersion(projectID, userID, versionID)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) ListProjectShareLinks(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	resp, err := h.serviceFor(c).ListProjectShareLinks(projectID, userID)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) CreateProjectShareLink(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	req := new(dto.CreateProjectShareLinkRequest)
	if err := c.Bind(req); err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid body")
	}
	link, err := h.serviceFor(c).CreateProjectShareLink(projectID, userID, *req, frontendBaseURL(c))
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusCreated, link)
}

func (h *Handler) AcceptProjectShareLink(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid share link token")
	}
	resp, err := h.serviceFor(c).AcceptProjectShareLink(token, userID)
	if err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) RevokeProjectShareLink(c echo.Context) error {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		return sendError(c, http.StatusUnauthorized, "unauthorized", err.Error())
	}
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid project UUID")
	}
	linkID, err := uuid.Parse(c.Param("linkId"))
	if err != nil {
		return sendError(c, http.StatusBadRequest, "invalid_request", "invalid share link UUID")
	}
	if err := h.serviceFor(c).RevokeProjectShareLink(projectID, userID, linkID); err != nil {
		return sendProjectExperienceError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
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

func frontendBaseURL(c echo.Context) string {
	if baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(frontendBaseURLEnv)), "/"); baseURL != "" {
		return baseURL
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
	return proto + "://" + host
}

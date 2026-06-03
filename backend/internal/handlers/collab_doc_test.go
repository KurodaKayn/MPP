package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/contracts"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCollabDocumentHandlerTest(t *testing.T) (*gorm.DB, *CollabDocumentHandler) {
	t.Helper()

	db := setupHandlerTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&models.CollabDocument{},
		&models.CollabDocumentCollaborator{},
	))

	return db, NewCollabDocumentHandler(services.NewCollabDocumentService(db))
}

func TestCollabDocumentHandlerCreateDocument(t *testing.T) {
	e := echo.New()
	db, handler := setupCollabDocumentHandlerTest(t)
	user := models.User{Username: "collab-owner"}
	require.NoError(t, db.Create(&user).Error)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/collab/documents",
		strings.NewReader(`{"title":" Team Plan "}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, user.ID)

	require.NoError(t, handler.CreateDocument(c))
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp contracts.CollabDocument
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, user.ID, resp.OwnerUserId)
	require.Equal(t, "Team Plan", resp.Title)
	require.Equal(t, contracts.CollabDocumentStatus(models.CollabDocumentStatusActive), resp.Status)
	require.Equal(t, 1, resp.SchemaVersion)
	require.Equal(t, int64(0), resp.CurrentSeq)

	var persisted models.CollabDocument
	require.NoError(t, db.First(&persisted, "id = ?", resp.Id).Error)
	require.Equal(t, "Team Plan", persisted.Title)
	require.Equal(t, user.ID, persisted.OwnerUserID)
}

func TestCollabDocumentHandlerCreateDocumentRejectsBlankTitle(t *testing.T) {
	e := echo.New()
	_, handler := setupCollabDocumentHandlerTest(t)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/collab/documents",
		strings.NewReader(`{"title":"   "}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, uuid.New())

	require.NoError(t, handler.CreateDocument(c))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestCollabDocumentHandlerListDocuments(t *testing.T) {
	e := echo.New()
	db, handler := setupCollabDocumentHandlerTest(t)
	user := createCollabHandlerTestUser(t, db, "list-owner")
	other := createCollabHandlerTestUser(t, db, "list-other")
	owned := createCollabHandlerTestDocument(t, db, user.ID, "Owned Doc", time.Now().Add(2*time.Hour))
	shared := createCollabHandlerTestDocument(t, db, other.ID, "Shared Doc", time.Now().Add(time.Hour))
	hidden := createCollabHandlerTestDocument(t, db, other.ID, "Hidden Doc", time.Now().Add(3*time.Hour))
	require.NoError(t, db.Create(&models.CollabDocumentCollaborator{
		DocumentID: shared.ID,
		UserID:     user.ID,
		Role:       models.CollabDocumentRoleEditor,
		CreatedBy:  other.ID,
	}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/collab/documents?page=1&limit=10", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, user.ID)

	require.NoError(t, handler.ListDocuments(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp contracts.PaginationCollabDocuments
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 1, resp.Page)
	require.Equal(t, 10, resp.Limit)
	require.Equal(t, 2, resp.Total)
	require.Equal(t, 1, resp.TotalPages)
	require.Len(t, resp.Items, 2)

	ids := map[uuid.UUID]bool{}
	for _, item := range resp.Items {
		ids[item.Id] = true
	}
	require.True(t, ids[owned.ID])
	require.True(t, ids[shared.ID])
	require.False(t, ids[hidden.ID])
}

func createCollabHandlerTestUser(t *testing.T, db *gorm.DB, username string) models.User {
	t.Helper()

	user := models.User{
		Username:     username,
		Email:        username + "@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func createCollabHandlerTestDocument(t *testing.T, db *gorm.DB, ownerUserID uuid.UUID, title string, updatedAt time.Time) models.CollabDocument {
	t.Helper()

	document := models.CollabDocument{
		OwnerUserID:   ownerUserID,
		Title:         title,
		Status:        models.CollabDocumentStatusActive,
		SchemaVersion: 1,
		UpdatedAt:     updatedAt,
	}
	require.NoError(t, db.Create(&document).Error)
	require.NoError(t, db.Model(&document).Update("updated_at", updatedAt).Error)
	return document
}

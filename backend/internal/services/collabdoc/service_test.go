package collabdoc_test

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	collabdoc "github.com/kurodakayn/mpp-backend/internal/services/collabdoc"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCollabDocumentServiceTest(t *testing.T) (*gorm.DB, *collabdoc.Service) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.User{},
		&models.CollabDocument{},
		&models.CollabDocumentCollaborator{},
	))

	return db, collabdoc.NewService(db)
}

func TestCreateDocumentPersistsOwnedActiveDocument(t *testing.T) {
	db, service := setupCollabDocumentServiceTest(t)
	owner := models.User{
		Username:     "doc-owner",
		Email:        "doc-owner@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, db.Create(&owner).Error)

	document, err := service.CreateDocument(context.Background(), owner.ID, "  Team Draft  ")

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, document.ID)
	require.Equal(t, owner.ID, document.OwnerUserID)
	require.Equal(t, "Team Draft", document.Title)
	require.Equal(t, models.CollabDocumentStatusActive, document.Status)
	require.Equal(t, 1, document.SchemaVersion)
	require.Equal(t, int64(0), document.CurrentSeq)
	require.False(t, document.CreatedAt.IsZero())
	require.False(t, document.UpdatedAt.IsZero())

	var persisted models.CollabDocument
	require.NoError(t, db.First(&persisted, "id = ?", document.ID).Error)
	require.Equal(t, document.Title, persisted.Title)
	require.Equal(t, document.OwnerUserID, persisted.OwnerUserID)
}

func TestCreateDocumentRejectsInvalidInput(t *testing.T) {
	_, service := setupCollabDocumentServiceTest(t)

	_, err := service.CreateDocument(context.Background(), uuid.Nil, "Team Draft")
	require.ErrorIs(t, err, collabdoc.ErrInvalidDocument)

	_, err = service.CreateDocument(context.Background(), uuid.New(), "   ")
	require.ErrorIs(t, err, collabdoc.ErrInvalidDocument)
}

func TestListDocumentsIncludesOwnedAndCollaborativeDocuments(t *testing.T) {
	db, service := setupCollabDocumentServiceTest(t)
	user := createCollabTestUser(t, db, "list-user")
	other := createCollabTestUser(t, db, "other-user")

	owned := createCollabTestDocument(t, db, user.ID, "Owned", time.Now().Add(2*time.Hour))
	shared := createCollabTestDocument(t, db, other.ID, "Shared", time.Now().Add(time.Hour))
	inaccessible := createCollabTestDocument(t, db, other.ID, "Hidden", time.Now().Add(3*time.Hour))
	require.NoError(t, db.Create(&models.CollabDocumentCollaborator{
		DocumentID: shared.ID,
		UserID:     user.ID,
		Role:       models.CollabDocumentRoleViewer,
		CreatedBy:  other.ID,
	}).Error)

	result, err := service.ListDocuments(context.Background(), user.ID, 1, 20)

	require.NoError(t, err)
	require.Equal(t, 1, result.Page)
	require.Equal(t, 20, result.Limit)
	require.Equal(t, int64(2), result.Total)
	require.Equal(t, 1, result.TotalPages)
	require.Len(t, result.Items, 2)
	require.Equal(t, owned.ID, result.Items[0].ID)
	require.Equal(t, shared.ID, result.Items[1].ID)
	require.NotEqual(t, inaccessible.ID, result.Items[0].ID)
	require.NotEqual(t, inaccessible.ID, result.Items[1].ID)
}

func TestListDocumentsPaginatesResults(t *testing.T) {
	db, service := setupCollabDocumentServiceTest(t)
	user := createCollabTestUser(t, db, "pagination-user")
	createCollabTestDocument(t, db, user.ID, "First", time.Now().Add(3*time.Hour))
	second := createCollabTestDocument(t, db, user.ID, "Second", time.Now().Add(2*time.Hour))
	createCollabTestDocument(t, db, user.ID, "Third", time.Now().Add(time.Hour))

	result, err := service.ListDocuments(context.Background(), user.ID, 2, 1)

	require.NoError(t, err)
	require.Equal(t, 2, result.Page)
	require.Equal(t, 1, result.Limit)
	require.Equal(t, int64(3), result.Total)
	require.Equal(t, 3, result.TotalPages)
	require.Len(t, result.Items, 1)
	require.Equal(t, second.ID, result.Items[0].ID)
}

func TestListDocumentsRejectsInvalidUser(t *testing.T) {
	_, service := setupCollabDocumentServiceTest(t)

	_, err := service.ListDocuments(context.Background(), uuid.Nil, 1, 20)

	require.ErrorIs(t, err, collabdoc.ErrInvalidDocument)
}

func createCollabTestUser(t *testing.T, db *gorm.DB, username string) models.User {
	t.Helper()

	user := models.User{
		Username:     username,
		Email:        username + "@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func createCollabTestDocument(t *testing.T, db *gorm.DB, ownerUserID uuid.UUID, title string, updatedAt time.Time) models.CollabDocument {
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

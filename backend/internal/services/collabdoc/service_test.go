package collabdoc_test

import (
	"context"
	"testing"

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

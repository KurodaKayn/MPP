package project_test

import (
	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCreateProjectCollabSessionLazilyLinksDocumentAndMapsRoles(t *testing.T) {
	db := testsupport.SetupTestDB()
	collabService := services.NewCollabDocumentService(db)
	collabService.UseSessionConfig(services.CollabDocumentSessionConfig{
		TokenSecret:      []byte("collab-secret"),
		WebsocketURLBase: "ws://collab.test",
	})
	initializer := &testsupport.FakeProjectDocumentInitializer{}
	collabService.UseProjectDocumentInitializer(initializer)
	s := services.NewDashboardService(db)
	s.SetCollabDocumentService(collabService)

	owner := models.User{Username: "collab-owner", Email: "collab-owner@example.com"}
	editor := models.User{Username: "collab-editor", Email: "collab-editor@example.com"}
	viewer := models.User{Username: "collab-viewer", Email: "collab-viewer@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&editor).Error)
	require.NoError(t, db.Create(&viewer).Error)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Realtime project",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    editor.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: owner.ID,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    viewer.ID,
		Role:      models.ProjectRoleViewer,
		CreatedBy: owner.ID,
	}).Error)

	ownerSession, err := s.CreateProjectCollabSession(project.ID, owner.ID)
	require.NoError(t, err)
	require.Equal(t, models.CollabDocumentRoleEditor, ownerSession.Role)
	require.Equal(t, "ws://collab.test/collab/documents/"+ownerSession.DocumentID.String(), ownerSession.WebsocketURL)

	editorSession, err := s.CreateProjectCollabSession(project.ID, editor.ID)
	require.NoError(t, err)
	require.Equal(t, ownerSession.DocumentID, editorSession.DocumentID)
	require.Equal(t, models.CollabDocumentRoleEditor, editorSession.Role)

	viewerSession, err := s.CreateProjectCollabSession(project.ID, viewer.ID)
	require.NoError(t, err)
	require.Equal(t, ownerSession.DocumentID, viewerSession.DocumentID)
	require.Equal(t, models.CollabDocumentRoleViewer, viewerSession.Role)
	require.Equal(t, []uuid.UUID{
		ownerSession.DocumentID,
		ownerSession.DocumentID,
		ownerSession.DocumentID,
	}, initializer.DocumentIDs)

	var savedProject models.Project
	require.NoError(t, db.First(&savedProject, "id = ?", project.ID).Error)
	require.NotNil(t, savedProject.CollabDocumentID)
	require.Equal(t, ownerSession.DocumentID, *savedProject.CollabDocumentID)

	detail, err := s.GetProject(project.ID, &owner.ID)
	require.NoError(t, err)
	require.NotNil(t, detail.CollabDocumentID)
	require.Equal(t, ownerSession.DocumentID, *detail.CollabDocumentID)

	var document models.CollabDocument
	require.NoError(t, db.First(&document, "id = ?", ownerSession.DocumentID).Error)
	require.Equal(t, owner.ID, document.OwnerUserID)
	require.Equal(t, project.Title, document.Title)

	var documentCount int64
	require.NoError(t, db.Model(&models.CollabDocument{}).Count(&documentCount).Error)
	require.Equal(t, int64(1), documentCount)
}

func TestCreateProjectCollabSessionRejectsNonCollaboratorWithoutCreatingDocument(t *testing.T) {
	db := testsupport.SetupTestDB()
	collabService := services.NewCollabDocumentService(db)
	collabService.UseSessionConfig(services.CollabDocumentSessionConfig{TokenSecret: []byte("collab-secret")})
	initializer := &testsupport.FakeProjectDocumentInitializer{}
	collabService.UseProjectDocumentInitializer(initializer)
	s := services.NewDashboardService(db)
	s.SetCollabDocumentService(collabService)

	owner := models.User{Username: "session-owner", Email: "session-owner@example.com"}
	stranger := models.User{Username: "session-stranger", Email: "session-stranger@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&stranger).Error)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Private project",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)

	_, err := s.CreateProjectCollabSession(project.ID, stranger.ID)
	require.ErrorIs(t, err, services.ErrForbidden)

	var documentCount int64
	require.NoError(t, db.Model(&models.CollabDocument{}).Count(&documentCount).Error)
	require.Zero(t, documentCount)
	require.Empty(t, initializer.DocumentIDs)
}

package project

import (
	"errors"
	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	collabdoc "github.com/kurodakayn/mpp-backend/internal/services/collabdoc"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func projectCollabDocumentRole(role string) (string, error) {
	switch role {
	case models.ProjectRoleOwner, models.ProjectRoleEditor:
		return models.CollabDocumentRoleEditor, nil
	case models.ProjectRoleViewer:
		return models.CollabDocumentRoleViewer, nil
	default:
		return "", ErrForbidden
	}
}

func (s *Service) CreateProjectCollabSession(projectID uuid.UUID, userID uuid.UUID) (*collabdoc.Session, error) {
	if projectID == uuid.Nil || userID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	if s.collabDocuments == nil {
		return nil, ErrProjectCollabUnavailable
	}

	documentID, documentRole, err := s.ensureProjectCollabDocument(projectID, userID)
	if err != nil {
		return nil, err
	}

	if err := s.collabDocuments.InitializeProjectDocument(s.requestContext(), documentID); err != nil {
		return nil, errors.Join(ErrProjectCollabUnavailable, err)
	}

	return s.collabDocuments.CreateAuthorizedSession(s.requestContext(), userID, documentID, documentRole)
}

func (s *Service) SyncProjectCollabSourceContent(projectID uuid.UUID, userID uuid.UUID) error {
	if projectID == uuid.Nil || userID == uuid.Nil {
		return ErrInvalidProject
	}

	var project models.Project
	if err := s.db.Select("id", "user_id", "workspace_id", "collab_document_id").First(&project, "id = ?", projectID).Error; err != nil {
		return err
	}
	role, err := s.ProjectAccessRole(project, userID)
	if err != nil {
		return err
	}
	if !CanEditProjectRole(role) {
		return ErrForbidden
	}

	return s.syncProjectSourceContentDocument(project.CollabDocumentID)
}

func (s *Service) SyncProjectCollabSourceContentIfMaterialized(projectID uuid.UUID, userID uuid.UUID) (bool, error) {
	if projectID == uuid.Nil || userID == uuid.Nil {
		return false, ErrInvalidProject
	}

	var project models.Project
	if err := s.db.Select("id", "user_id", "workspace_id", "collab_document_id").First(&project, "id = ?", projectID).Error; err != nil {
		return false, err
	}
	role, err := s.ProjectAccessRole(project, userID)
	if err != nil {
		return false, err
	}
	if !CanEditProjectRole(role) {
		return false, ErrForbidden
	}

	return s.syncProjectSourceContentDocumentIfMaterialized(project.CollabDocumentID)
}

func (s *Service) syncProjectSourceContentDocument(documentID *uuid.UUID) error {
	if documentID == nil || *documentID == uuid.Nil {
		return nil
	}
	if s.collabDocuments == nil {
		return ErrProjectCollabUnavailable
	}
	if err := s.collabDocuments.SyncProjectSourceContent(s.requestContext(), *documentID); err != nil {
		return errors.Join(ErrProjectCollabUnavailable, err)
	}
	return nil
}

func (s *Service) syncProjectSourceContentDocumentIfMaterialized(documentID *uuid.UUID) (bool, error) {
	if documentID == nil || *documentID == uuid.Nil {
		return false, nil
	}

	materialized, err := s.projectCollabDocumentHasMaterializedState(*documentID)
	if err != nil {
		return false, err
	}
	if !materialized {
		return false, nil
	}

	return true, s.syncProjectSourceContentDocument(documentID)
}

func (s *Service) projectCollabDocumentHasMaterializedState(documentID uuid.UUID) (bool, error) {
	var document models.CollabDocument
	if err := s.db.Select("id", "current_seq").First(&document, "id = ?", documentID).Error; err != nil {
		return false, err
	}
	if document.CurrentSeq != 0 {
		return true, nil
	}

	var count int64
	if err := s.db.Model(&models.CollabDocumentState{}).Where("document_id = ?", documentID).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}

	if err := s.db.Model(&models.CollabDocumentUpdateBatch{}).Where("document_id = ?", documentID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Service) ensureProjectCollabDocument(projectID uuid.UUID, userID uuid.UUID) (uuid.UUID, string, error) {
	var documentID uuid.UUID
	var documentRole string
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var project models.Project
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, "id = ?", projectID).Error; err != nil {
			return err
		}

		role, err := ProjectAccessRoleWithDB(tx, project, userID)
		if err != nil {
			return err
		}
		documentRole, err = projectCollabDocumentRole(role)
		if err != nil {
			return err
		}

		if project.CollabDocumentID != nil && *project.CollabDocumentID != uuid.Nil {
			documentID = *project.CollabDocumentID
			return nil
		}

		document := models.CollabDocument{
			OwnerUserID:   project.UserID,
			Title:         project.Title,
			Status:        models.CollabDocumentStatusActive,
			SchemaVersion: 1,
			CurrentSeq:    0,
		}
		if err := tx.Create(&document).Error; err != nil {
			return err
		}
		if err := tx.Model(&project).Update("collab_document_id", document.ID).Error; err != nil {
			return err
		}
		documentID = document.ID
		return nil
	})
	return documentID, documentRole, err
}

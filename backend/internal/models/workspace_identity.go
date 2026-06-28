package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var personalWorkspaceNamespace = uuid.MustParse("03d32585-3f8c-48a8-bf40-53aa3f1698c1")

func PersonalWorkspaceID(userID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(personalWorkspaceNamespace, []byte(userID.String()))
}

func PersonalWorkspaceSlug(userID uuid.UUID) string {
	return "personal-" + userID.String()
}

func projectWorkspaceIDValue(project Project) uuid.UUID {
	if project.WorkspaceID != nil && *project.WorkspaceID != uuid.Nil {
		return *project.WorkspaceID
	}
	if project.UserID != uuid.Nil {
		return PersonalWorkspaceID(project.UserID)
	}
	return uuid.Nil
}

func ProjectWorkspaceID(project Project) uuid.UUID {
	return projectWorkspaceIDValue(project)
}

func deriveWorkspaceIDFromProject(db *gorm.DB, projectID uuid.UUID, fallbackUserID uuid.UUID) uuid.UUID {
	if projectID == uuid.Nil {
		if fallbackUserID != uuid.Nil {
			return PersonalWorkspaceID(fallbackUserID)
		}
		return uuid.Nil
	}

	var project Project
	if err := db.Select("id", "user_id", "workspace_id").First(&project, "id = ?", projectID).Error; err == nil {
		return projectWorkspaceIDValue(project)
	}
	if fallbackUserID != uuid.Nil {
		return PersonalWorkspaceID(fallbackUserID)
	}
	return uuid.Nil
}

func deriveWorkspaceIDFromDocument(db *gorm.DB, documentID uuid.UUID) uuid.UUID {
	if documentID == uuid.Nil {
		return uuid.Nil
	}

	var document CollabDocument
	if err := db.Select("id", "workspace_id").First(&document, "id = ?", documentID).Error; err == nil {
		return document.WorkspaceID
	}
	return uuid.Nil
}

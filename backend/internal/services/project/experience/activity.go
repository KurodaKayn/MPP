package experience

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

func (s *Service) ListProjectActivities(projectID uuid.UUID, userID uuid.UUID, limit int) (*dto.ProjectActivitiesResponse, error) {
	project, err := s.accessibleProject(projectID, userID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var activities []models.ProjectActivity
	if err := s.db.
		Preload("Actor", selectUserIdentity).
		Preload("TargetUser", selectUserIdentity).
		Where("workspace_id = ? AND project_id = ?", models.ProjectWorkspaceID(project), projectID).
		Order("created_at desc").
		Order("id desc").
		Limit(limit).
		Find(&activities).Error; err != nil {
		return nil, err
	}

	items := make([]dto.ProjectActivity, 0, len(activities))
	for _, activity := range activities {
		items = append(items, projectActivityFromModel(activity))
	}
	return &dto.ProjectActivitiesResponse{Items: items}, nil
}

func RecordProjectActivity(tx *gorm.DB, projectID uuid.UUID, actorUserID uuid.UUID, targetUserID *uuid.UUID, eventType string, metadata map[string]any) error {
	if projectID == uuid.Nil || actorUserID == uuid.Nil || strings.TrimSpace(eventType) == "" {
		return nil
	}
	payload, err := JSONMap(metadata)
	if err != nil {
		return err
	}
	var project models.Project
	if err := tx.Select("id", "user_id", "workspace_id").First(&project, "id = ?", projectID).Error; err != nil {
		return err
	}
	workspaceID := models.ProjectWorkspaceID(project)
	createdAt := time.Now().UTC()
	var latestCreatedAt time.Time
	if err := tx.
		Model(&models.ProjectActivity{}).
		Where("workspace_id = ? AND project_id = ?", workspaceID, projectID).
		Select("created_at").
		Order("created_at desc").
		Limit(1).
		Scan(&latestCreatedAt).Error; err != nil {
		return err
	}
	if !latestCreatedAt.IsZero() && !createdAt.After(latestCreatedAt) {
		createdAt = latestCreatedAt.Add(time.Nanosecond)
	}
	return tx.Create(&models.ProjectActivity{
		WorkspaceID:  workspaceID,
		ProjectID:    projectID,
		ActorUserID:  actorUserID,
		TargetUserID: targetUserID,
		EventType:    eventType,
		Metadata:     payload,
		CreatedAt:    createdAt,
	}).Error
}

func projectActivityFromModel(activity models.ProjectActivity) dto.ProjectActivity {
	item := dto.ProjectActivity{
		ID:            activity.ID,
		ProjectID:     activity.ProjectID,
		ActorUserID:   activity.ActorUserID,
		ActorUsername: activity.Actor.Username,
		ActorEmail:    activity.Actor.Email,
		TargetUserID:  activity.TargetUserID,
		EventType:     activity.EventType,
		Metadata:      MapFromJSON(activity.Metadata),
		CreatedAt:     activity.CreatedAt,
	}
	if activity.TargetUser != nil {
		item.TargetUsername = activity.TargetUser.Username
		item.TargetEmail = activity.TargetUser.Email
	}
	return item
}

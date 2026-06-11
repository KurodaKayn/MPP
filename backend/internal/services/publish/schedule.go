package publish

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

func (s *Service) createScheduledPublication(ctx context.Context, project models.Project, pub models.ProjectPlatformPublication, userID uuid.UUID, idempotencyKey string, scheduledAt time.Time, status string) (models.ScheduledPublication, error) {
	if !s.db.Migrator().HasTable(&models.ScheduledPublication{}) {
		return models.ScheduledPublication{}, nil
	}
	workspaceID := models.PersonalWorkspaceID(project.UserID)
	if project.WorkspaceID != nil && *project.WorkspaceID != uuid.Nil {
		workspaceID = *project.WorkspaceID
	}
	schedule := models.ScheduledPublication{
		WorkspaceID:       workspaceID,
		ProjectID:         project.ID,
		PublicationID:     pub.ID,
		PlatformAccountID: pub.PlatformAccountID,
		ProjectVersionID:  nil,
		ScheduledAt:       scheduledAt.UTC(),
		Timezone:          "UTC",
		Status:            status,
		IdempotencyKey:    idempotencyKey,
		CreatedBy:         userID,
	}
	if schedule.Status == "" {
		schedule.Status = models.ScheduledPublicationStatusScheduled
	}
	if latestVersionID, err := s.latestProjectVersionID(ctx, project.ID); err != nil {
		return models.ScheduledPublication{}, err
	} else if latestVersionID != uuid.Nil {
		schedule.ProjectVersionID = &latestVersionID
	}
	db := s.db
	if ctx != nil {
		db = db.WithContext(ctx)
	}
	if err := db.Create(&schedule).Error; err != nil {
		return models.ScheduledPublication{}, err
	}
	return schedule, nil
}

func (s *Service) latestProjectVersionID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	if !s.db.Migrator().HasTable(&models.ProjectVersion{}) {
		return uuid.Nil, nil
	}
	db := s.db
	if ctx != nil {
		db = db.WithContext(ctx)
	}
	var version models.ProjectVersion
	err := db.Select("id").
		Where("project_id = ?", projectID).
		Order("version_number DESC, created_at DESC").
		First(&version).Error
	if err == nil {
		return version.ID, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return uuid.Nil, nil
	}
	return uuid.Nil, err
}

func (s *Service) startPublishAttempt(scheduleID uuid.UUID, startedAt time.Time) (models.PublishAttempt, bool, error) {
	if scheduleID == uuid.Nil {
		return models.PublishAttempt{}, false, nil
	}
	if !s.db.Migrator().HasTable(&models.ScheduledPublication{}) || !s.db.Migrator().HasTable(&models.PublishAttempt{}) {
		return models.PublishAttempt{}, false, nil
	}
	var schedule models.ScheduledPublication
	if err := s.db.Select("id").First(&schedule, "id = ?", scheduleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.PublishAttempt{}, false, nil
		}
		return models.PublishAttempt{}, false, err
	}
	var attemptNo int
	if err := s.db.Model(&models.PublishAttempt{}).
		Where("scheduled_publication_id = ?", scheduleID).
		Select("COALESCE(MAX(attempt_no), 0)").
		Scan(&attemptNo).Error; err != nil {
		return models.PublishAttempt{}, false, err
	}
	attempt := models.PublishAttempt{
		ScheduledPublicationID: scheduleID,
		AttemptNo:              attemptNo + 1,
		StartedAt:              startedAt,
		Status:                 models.PublishAttemptStatusRunning,
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.ScheduledPublication{}).
			Where("id = ?", scheduleID).
			Updates(map[string]any{
				"status":     models.ScheduledPublicationStatusRunning,
				"last_error": "",
			}).Error; err != nil {
			return err
		}
		return tx.Create(&attempt).Error
	}); err != nil {
		return models.PublishAttempt{}, false, err
	}
	return attempt, true, nil
}

func (s *Service) finishPublishAttempt(attempt *models.PublishAttempt, status string, remoteID string, publishURL string, errorMessage string) error {
	if attempt == nil {
		return nil
	}
	finishedAt := time.Now().UTC()
	scheduleStatus := models.ScheduledPublicationStatusPublished
	errorCode := ""
	if status != models.PublishAttemptStatusSucceeded {
		scheduleStatus = models.ScheduledPublicationStatusFailed
		errorCode = status
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.PublishAttempt{}).
			Where("id = ?", attempt.ID).
			Updates(map[string]any{
				"finished_at":   &finishedAt,
				"status":        status,
				"remote_id":     remoteID,
				"publish_url":   publishURL,
				"error_code":    errorCode,
				"error_message": errorMessage,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&models.ScheduledPublication{}).
			Where("id = ?", attempt.ScheduledPublicationID).
			Updates(map[string]any{
				"status":     scheduleStatus,
				"last_error": errorMessage,
			}).Error
	})
}

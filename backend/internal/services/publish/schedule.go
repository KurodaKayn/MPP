package publish

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

const (
	scheduledPublicationPollInterval = 5 * time.Second
	scheduledPublicationBatchSize    = 25
)

var (
	errScheduledPublicationMissing      = errors.New("scheduled publication does not exist")
	errScheduledPublicationNotStartable = errors.New("scheduled publication is not startable")
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

func (s *Service) ScheduleProjectPublication(ctx context.Context, projectID uuid.UUID, userID uuid.UUID, req dto.SchedulePublicationRequest) (*dto.ScheduledPublication, error) {
	if req.ScheduledAt.IsZero() || strings.TrimSpace(req.Platform) == "" {
		return nil, ErrPublicationRequiresSync
	}
	project, pub, err := s.preparePublishJob(ctx, projectID, req.Platform, userID)
	if err != nil {
		return nil, err
	}
	timezone := strings.TrimSpace(req.Timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	idempotencyKey := normalizeIdempotencyKey(req.IdempotencyKey)
	if idempotencyKey != "" {
		if existing, found, err := s.findIdempotentScheduledPublication(ctx, project.ID, pub.ID, userID, idempotencyKey); err != nil {
			return nil, err
		} else if found {
			item := scheduledPublicationFromModel(existing, project, pub, nil)
			return &item, nil
		}
	}
	schedule, err := s.createScheduledPublication(ctx, project, pub, userID, idempotencyKey, req.ScheduledAt, models.ScheduledPublicationStatusScheduled)
	if err != nil {
		return nil, err
	}
	if schedule.ID == uuid.Nil {
		return nil, ErrPublicationRequiresSync
	}
	if timezone != schedule.Timezone {
		if err := s.db.WithContext(ctx).Model(&schedule).Update("timezone", timezone).Error; err != nil {
			return nil, err
		}
		schedule.Timezone = timezone
	}
	item := scheduledPublicationFromModel(schedule, project, pub, nil)
	return &item, nil
}

func (s *Service) findIdempotentScheduledPublication(ctx context.Context, projectID uuid.UUID, publicationID uuid.UUID, userID uuid.UUID, key string) (models.ScheduledPublication, bool, error) {
	if strings.TrimSpace(key) == "" {
		return models.ScheduledPublication{}, false, nil
	}
	db := s.db
	if ctx != nil {
		db = db.WithContext(ctx)
	}
	var schedule models.ScheduledPublication
	err := db.
		Where("project_id = ? AND publication_id = ? AND created_by = ? AND idempotency_key = ?", projectID, publicationID, userID, key).
		Order("created_at DESC").
		First(&schedule).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.ScheduledPublication{}, false, nil
	}
	if err != nil {
		return models.ScheduledPublication{}, false, err
	}
	return schedule, true, nil
}

func (s *Service) CancelScheduledPublication(ctx context.Context, projectID uuid.UUID, scheduleID uuid.UUID, userID uuid.UUID) (*dto.ScheduledPublication, error) {
	if projectID == uuid.Nil || scheduleID == uuid.Nil || userID == uuid.Nil {
		return nil, ErrForbidden
	}
	var schedule models.ScheduledPublication
	if err := s.db.WithContext(ctx).
		Preload("Project").
		Preload("Publication").
		First(&schedule, "id = ? AND project_id = ?", scheduleID, projectID).Error; err != nil {
		return nil, err
	}
	if _, err := s.projectForPublish(ctx, schedule.ProjectID, userID); err != nil {
		return nil, err
	}
	if schedule.Status == models.ScheduledPublicationStatusRunning || schedule.Status == models.ScheduledPublicationStatusPublished {
		return nil, ErrPublicationAlreadyPublishing
	}
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Model(&schedule).Updates(map[string]any{
		"status":       models.ScheduledPublicationStatusCancelled,
		"cancelled_by": userID,
		"updated_at":   now,
	}).Error; err != nil {
		return nil, err
	}
	schedule.Status = models.ScheduledPublicationStatusCancelled
	schedule.CancelledBy = &userID
	item := scheduledPublicationFromModel(schedule, schedule.Project, schedule.Publication, nil)
	return &item, nil
}

func (s *Service) RetryScheduledPublication(ctx context.Context, projectID uuid.UUID, scheduleID uuid.UUID, userID uuid.UUID) (*dto.ScheduledPublication, error) {
	if projectID == uuid.Nil || scheduleID == uuid.Nil || userID == uuid.Nil {
		return nil, ErrForbidden
	}
	var schedule models.ScheduledPublication
	if err := s.db.WithContext(ctx).
		Preload("Project").
		Preload("Publication").
		First(&schedule, "id = ? AND project_id = ?", scheduleID, projectID).Error; err != nil {
		return nil, err
	}
	if schedule.Status != models.ScheduledPublicationStatusFailed && schedule.Status != models.ScheduledPublicationStatusNeedsManualAction {
		return nil, ErrPublicationAlreadyPublishing
	}
	if _, err := s.PublishProjectWithContext(ctx, projectID, schedule.Publication.Platform, &userID, scheduleID); err != nil {
		return nil, err
	}
	return s.scheduledPublicationDetail(ctx, scheduleID)
}

func (s *Service) ListWorkspaceScheduledPublications(ctx context.Context, workspaceID uuid.UUID, userID uuid.UUID, from time.Time, to time.Time) (*dto.ScheduledPublicationsResponse, error) {
	if workspaceID == uuid.Nil || userID == uuid.Nil || from.IsZero() || to.IsZero() || !to.After(from) {
		return nil, ErrForbidden
	}
	if err := s.requireWorkspaceCalendarAccess(ctx, workspaceID, userID); err != nil {
		return nil, err
	}
	var schedules []models.ScheduledPublication
	if err := s.db.WithContext(ctx).
		Preload("Project").
		Preload("Publication").
		Where("workspace_id = ? AND scheduled_at >= ? AND scheduled_at < ?", workspaceID, from, to).
		Order("scheduled_at ASC").
		Find(&schedules).Error; err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(schedules))
	for _, schedule := range schedules {
		ids = append(ids, schedule.ID)
	}
	attemptsBySchedule := map[uuid.UUID][]models.PublishAttempt{}
	if len(ids) > 0 {
		var attempts []models.PublishAttempt
		if err := s.db.WithContext(ctx).
			Where("scheduled_publication_id IN ?", ids).
			Order("attempt_no ASC").
			Find(&attempts).Error; err != nil {
			return nil, err
		}
		for _, attempt := range attempts {
			attemptsBySchedule[attempt.ScheduledPublicationID] = append(attemptsBySchedule[attempt.ScheduledPublicationID], attempt)
		}
	}
	items := make([]dto.ScheduledPublication, 0, len(schedules))
	for _, schedule := range schedules {
		items = append(items, scheduledPublicationFromModel(schedule, schedule.Project, schedule.Publication, attemptsBySchedule[schedule.ID]))
	}
	return &dto.ScheduledPublicationsResponse{Items: items}, nil
}

func (s *Service) scheduledPublicationDetail(ctx context.Context, scheduleID uuid.UUID) (*dto.ScheduledPublication, error) {
	var schedule models.ScheduledPublication
	if err := s.db.WithContext(ctx).
		Preload("Project").
		Preload("Publication").
		First(&schedule, "id = ?", scheduleID).Error; err != nil {
		return nil, err
	}
	var attempts []models.PublishAttempt
	if err := s.db.WithContext(ctx).
		Where("scheduled_publication_id = ?", scheduleID).
		Order("attempt_no ASC").
		Find(&attempts).Error; err != nil {
		return nil, err
	}
	item := scheduledPublicationFromModel(schedule, schedule.Project, schedule.Publication, attempts)
	return &item, nil
}

func (s *Service) requireWorkspaceCalendarAccess(ctx context.Context, workspaceID uuid.UUID, userID uuid.UUID) error {
	db := s.db
	if ctx != nil {
		db = db.WithContext(ctx)
	}
	var workspace models.Workspace
	if err := db.Select("id", "owner_user_id").First(&workspace, "id = ?", workspaceID).Error; err != nil {
		return err
	}
	if workspace.OwnerUserID == userID {
		return nil
	}
	var member models.WorkspaceMember
	if err := db.Select("workspace_id", "user_id").First(&member, "workspace_id = ? AND user_id = ?", workspaceID, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrForbidden
		}
		return err
	}
	return nil
}

func (s *Service) StartScheduledPublicationDispatcher(ctx context.Context) {
	if s.queue == nil || s.db == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(scheduledPublicationPollInterval)
		defer ticker.Stop()

		for {
			if err := s.FlushScheduledPublications(ctx, scheduledPublicationBatchSize); err != nil && ctx.Err() == nil {
				log.Printf("scheduled publication flush failed: %v", err)
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *Service) FlushScheduledPublications(ctx context.Context, limit int) error {
	if s.queue == nil || s.db == nil {
		return nil
	}
	if limit <= 0 {
		limit = scheduledPublicationBatchSize
	}

	now := time.Now().UTC()
	var schedules []models.ScheduledPublication
	if err := s.db.WithContext(ctx).
		Preload("Publication").
		Where("status = ? AND scheduled_at <= ?", models.ScheduledPublicationStatusScheduled, now).
		Order("scheduled_at ASC").
		Limit(limit).
		Find(&schedules).Error; err != nil {
		return err
	}

	var firstErr error
	for _, schedule := range schedules {
		if err := s.dispatchScheduledPublication(ctx, schedule); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *Service) dispatchScheduledPublication(ctx context.Context, schedule models.ScheduledPublication) error {
	if schedule.ID == uuid.Nil || schedule.ProjectID == uuid.Nil || schedule.CreatedBy == uuid.Nil || schedule.PublicationID == uuid.Nil {
		return nil
	}
	if schedule.Status != models.ScheduledPublicationStatusScheduled || schedule.ScheduledAt.After(time.Now().UTC()) {
		return nil
	}
	platform := strings.TrimSpace(schedule.Publication.Platform)
	if platform == "" {
		var publication models.ProjectPlatformPublication
		if err := s.db.WithContext(ctx).Select("platform").First(&publication, "id = ?", schedule.PublicationID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		platform = publication.Platform
	}
	if platform == "" {
		return nil
	}

	job := PublishJob{
		JobID:          uuid.New(),
		ProjectID:      schedule.ProjectID,
		UserID:         schedule.CreatedBy,
		Platform:       platform,
		PublicationID:  schedule.PublicationID,
		ScheduleID:     schedule.ID,
		IdempotencyKey: schedule.IdempotencyKey,
		EnqueuedAt:     time.Now().UTC(),
	}
	lockKey := publishLockKey(schedule.ProjectID, platform)
	acquired, err := s.queue.AcquireLock(ctx, lockKey, job.JobID.String(), publishLockTTL)
	if err != nil {
		return err
	}
	if !acquired {
		return nil
	}
	return s.processPublishJob(ctx, job)
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
	var attempt models.PublishAttempt
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.ScheduledPublication{}).
			Where("id = ? AND status IN ?", scheduleID, []string{
				models.ScheduledPublicationStatusScheduled,
				models.ScheduledPublicationStatusFailed,
				models.ScheduledPublicationStatusNeedsManualAction,
			}).
			Updates(map[string]any{
				"status":     models.ScheduledPublicationStatusRunning,
				"last_error": "",
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var schedule models.ScheduledPublication
			if err := tx.Select("status").First(&schedule, "id = ?", scheduleID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errScheduledPublicationMissing
				}
				return err
			}
			return fmt.Errorf("%w: %s", errScheduledPublicationNotStartable, schedule.Status)
		}

		var attemptNo int
		if err := tx.Model(&models.PublishAttempt{}).
			Where("scheduled_publication_id = ?", scheduleID).
			Select("COALESCE(MAX(attempt_no), 0)").
			Scan(&attemptNo).Error; err != nil {
			return err
		}

		attempt = models.PublishAttempt{
			ScheduledPublicationID: scheduleID,
			AttemptNo:              attemptNo + 1,
			StartedAt:              startedAt,
			Status:                 models.PublishAttemptStatusRunning,
		}
		return tx.Create(&attempt).Error
	}); err != nil {
		if errors.Is(err, errScheduledPublicationMissing) {
			return models.PublishAttempt{}, false, nil
		}
		if errors.Is(err, errScheduledPublicationNotStartable) {
			return models.PublishAttempt{}, false, ErrPublicationAlreadyPublishing
		}
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

func scheduledPublicationFromModel(schedule models.ScheduledPublication, project models.Project, pub models.ProjectPlatformPublication, attempts []models.PublishAttempt) dto.ScheduledPublication {
	dtoAttempts := make([]dto.PublishAttempt, 0, len(attempts))
	for _, attempt := range attempts {
		dtoAttempts = append(dtoAttempts, dto.PublishAttempt{
			ID:                     attempt.ID,
			ScheduledPublicationID: attempt.ScheduledPublicationID,
			AttemptNo:              attempt.AttemptNo,
			StartedAt:              attempt.StartedAt,
			FinishedAt:             attempt.FinishedAt,
			Status:                 attempt.Status,
			RemoteID:               attempt.RemoteID,
			PublishURL:             attempt.PublishURL,
			ErrorCode:              attempt.ErrorCode,
			ErrorMessage:           attempt.ErrorMessage,
		})
	}
	return dto.ScheduledPublication{
		ID:                schedule.ID,
		WorkspaceID:       schedule.WorkspaceID,
		ProjectID:         schedule.ProjectID,
		PublicationID:     schedule.PublicationID,
		PlatformAccountID: schedule.PlatformAccountID,
		ProjectVersionID:  schedule.ProjectVersionID,
		Platform:          pub.Platform,
		ProjectTitle:      project.Title,
		ScheduledAt:       schedule.ScheduledAt,
		Timezone:          schedule.Timezone,
		Status:            schedule.Status,
		IdempotencyKey:    schedule.IdempotencyKey,
		CreatedBy:         schedule.CreatedBy,
		ApprovedBy:        schedule.ApprovedBy,
		CancelledBy:       schedule.CancelledBy,
		LastError:         schedule.LastError,
		ManualActionURL:   schedule.ManualActionURL,
		ManualActionUntil: schedule.ManualActionUntil,
		Attempts:          dtoAttempts,
		CreatedAt:         schedule.CreatedAt,
		UpdatedAt:         schedule.UpdatedAt,
	}
}

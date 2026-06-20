package publicationselection

import (
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

type ConfigForPlatform func(platform string) (datatypes.JSON, error)

type ReconcileMode int

const (
	ReconcileKeepActive ReconcileMode = iota
	ReconcileResetAll
)

func CreateSelected(tx *gorm.DB, projectID uuid.UUID, platforms []string, configForPlatform ConfigForPlatform) ([]models.ProjectPlatformPublication, error) {
	publications := make([]models.ProjectPlatformPublication, 0, len(platforms))
	for _, platform := range platforms {
		config, err := configForPlatform(platform)
		if err != nil {
			return nil, err
		}
		publication := createPendingPublication(projectID, platform, config)
		if err := tx.Create(&publication).Error; err != nil {
			return nil, err
		}
		publications = append(publications, publication)
	}
	return publications, nil
}

func ReconcileSelected(tx *gorm.DB, projectID uuid.UUID, platforms []string, mode ReconcileMode, configForPlatform ConfigForPlatform) ([]models.ProjectPlatformPublication, error) {
	var existing []models.ProjectPlatformPublication
	if err := tx.Where("project_id = ?", projectID).Find(&existing).Error; err != nil {
		return nil, err
	}

	selected := make(map[string]struct{}, len(platforms))
	for _, platform := range platforms {
		selected[platform] = struct{}{}
	}

	for _, publication := range existing {
		if _, ok := selected[publication.Platform]; !ok {
			if err := disablePublication(tx, publication).Error; err != nil {
				return nil, err
			}
			continue
		}

		if mode == ReconcileResetAll || !publication.Enabled || publication.Status == models.PublicationStatusDisabled {
			config, err := configForPlatform(publication.Platform)
			if err != nil {
				return nil, err
			}
			if err := resetPublicationForDraft(tx, publication, config, mode).Error; err != nil {
				return nil, err
			}
		}
		delete(selected, publication.Platform)
	}

	for _, platform := range platforms {
		if _, ok := selected[platform]; !ok {
			continue
		}
		config, err := configForPlatform(platform)
		if err != nil {
			return nil, err
		}
		publication := createPendingPublication(projectID, platform, config)
		if err := tx.Create(&publication).Error; err != nil {
			return nil, err
		}
	}

	var publications []models.ProjectPlatformPublication
	if err := tx.Where("project_id = ?", projectID).Find(&publications).Error; err != nil {
		return nil, err
	}
	return publications, nil
}

func MarkDraftsStale(tx *gorm.DB, projectID uuid.UUID) error {
	return tx.Model(&models.ProjectPlatformPublication{}).
		Where("project_id = ? AND enabled = ? AND status NOT IN ?", projectID, true, []string{
			models.PublicationStatusQueued,
			models.PublicationStatusPublishing,
			models.PublicationStatusSucceeded,
		}).
		Updates(map[string]any{
			"draft_status":  models.PublicationDraftStatusStale,
			"review_status": models.PublicationReviewStatusDraft,
			"sync_required": true,
		}).Error
}

func createPendingPublication(projectID uuid.UUID, platform string, config datatypes.JSON) models.ProjectPlatformPublication {
	return models.ProjectPlatformPublication{
		ProjectID:      projectID,
		Platform:       platform,
		Enabled:        true,
		Status:         models.PublicationStatusPending,
		SyncRequired:   true,
		Config:         config,
		AdaptedContent: datatypes.JSON([]byte(`{}`)),
	}
}

func disablePublication(tx *gorm.DB, publication models.ProjectPlatformPublication) *gorm.DB {
	return tx.Model(&publication).Updates(map[string]any{
		"enabled":       false,
		"error_message": "",
		"status":        models.PublicationStatusDisabled,
	})
}

func resetPublicationForDraft(tx *gorm.DB, publication models.ProjectPlatformPublication, config datatypes.JSON, mode ReconcileMode) *gorm.DB {
	updates := map[string]any{
		"draft_status":  models.PublicationDraftStatusUnsynced,
		"enabled":       true,
		"review_status": models.PublicationReviewStatusDraft,
		"status":        models.PublicationStatusPending,
		"sync_required": true,
	}
	if mode == ReconcileResetAll {
		updates["config"] = config
		updates["error_message"] = ""
		updates["draft_status"] = models.PublicationDraftStatusStale
		updates["last_attempt_at"] = nil
		updates["published_at"] = nil
		updates["publish_url"] = ""
		updates["remote_id"] = ""
		updates["retry_count"] = 0
	}
	return tx.Model(&publication).Updates(updates)
}

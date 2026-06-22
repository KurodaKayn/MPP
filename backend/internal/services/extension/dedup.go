package extension

import (
	"errors"

	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

func loadClaimedExtensionEvent(tx *gorm.DB, eventID string, event *models.ExtensionExecutionEvent) error {
	var claim models.ExtensionExecutionEventClaim
	if err := tx.First(&claim, "event_id = ?", eventID).Error; err != nil {
		return err
	}
	if err := tx.First(event, "id = ?", claim.RecordID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	return nil
}

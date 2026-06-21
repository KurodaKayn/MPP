package extension

import "gorm.io/gorm"

func lockExtensionEventID(tx *gorm.DB, eventID string) error {
	if tx == nil || tx.Name() != "postgres" {
		return nil
	}
	return tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", eventID).Error
}

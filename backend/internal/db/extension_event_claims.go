package db

import (
	"fmt"

	"gorm.io/gorm"
)

func backfillExtensionExecutionEventClaims(database *gorm.DB) error {
	if !database.Migrator().HasTable("extension_execution_events") ||
		!database.Migrator().HasTable("extension_execution_event_claims") {
		return nil
	}

	switch database.Name() {
	case "postgres":
		return database.Exec(`
			INSERT INTO extension_execution_event_claims (event_id, record_id, created_at)
			SELECT DISTINCT ON (event_id) event_id, id, created_at
			FROM extension_execution_events
			ORDER BY event_id, created_at ASC, id ASC
			ON CONFLICT (event_id) DO NOTHING
		`).Error
	case "sqlite":
		return database.Exec(`
			INSERT OR IGNORE INTO extension_execution_event_claims (event_id, record_id, created_at)
			SELECT e.event_id, e.id, e.created_at
			FROM extension_execution_events e
			WHERE NOT EXISTS (
				SELECT 1
				FROM extension_execution_events older
				WHERE older.event_id = e.event_id
					AND (
						older.created_at < e.created_at
						OR (older.created_at = e.created_at AND older.id < e.id)
					)
			)
		`).Error
	default:
		return fmt.Errorf("unsupported database dialect %q for extension event claim backfill", database.Name())
	}
}

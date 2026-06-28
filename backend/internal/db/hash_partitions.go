package db

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const collabUpdateBatchHashPartitionCount = 16

type hashPartitionedTable struct {
	name           string
	partitionKey   string
	partitionCount int
	createSQL      string
	columns        []string
}

var collabUpdateBatchHashPartitionedTable = hashPartitionedTable{
	name:           "collab_document_update_batches",
	partitionKey:   "document_id",
	partitionCount: collabUpdateBatchHashPartitionCount,
	createSQL: `
		CREATE TABLE IF NOT EXISTS collab_document_update_batches (
			id bigserial NOT NULL,
			document_id uuid NOT NULL,
			workspace_id uuid NOT NULL,
			from_seq bigint NOT NULL,
			to_seq bigint NOT NULL,
			update_payload bytea NOT NULL,
			update_count integer NOT NULL,
			payload_size_bytes integer NOT NULL,
			actor_user_id uuid,
			created_at timestamptz NOT NULL,
			CONSTRAINT pk_collab_document_update_batches_partitioned PRIMARY KEY (id, document_id)
		) PARTITION BY HASH (document_id)
	`,
	columns: []string{
		"id",
		"document_id",
		"workspace_id",
		"from_seq",
		"to_seq",
		"update_payload",
		"update_count",
		"payload_size_bytes",
		"actor_user_id",
		"created_at",
	},
}

func ensureCollabUpdateBatchHashPartitions(database *gorm.DB) error {
	if database.Name() != "postgres" {
		return nil
	}
	return ensureHashPartitionedTable(database, collabUpdateBatchHashPartitionedTable)
}

func ensureHashPartitionedTable(database *gorm.DB, table hashPartitionedTable) error {
	partitionKey, exists, err := postgresPartitionedTableKey(database, table.name)
	if err != nil {
		return err
	}

	expectedKey := fmt.Sprintf("HASH (%s)", table.partitionKey)
	if exists && partitionKey != "" && !strings.EqualFold(partitionKey, expectedKey) {
		return fmt.Errorf("%s is partitioned by %s, expected %s", table.name, partitionKey, expectedKey)
	}

	if exists && partitionKey == "" {
		if err := migrateRegularTableToHashPartitions(database, table); err != nil {
			return err
		}
	} else if !exists {
		if err := database.Exec(table.createSQL).Error; err != nil {
			return err
		}
	}

	for partitionIndex := range table.partitionCount {
		if err := database.Exec(createHashPartitionSQL(table.name, partitionIndex, table.partitionCount)).Error; err != nil {
			return err
		}
	}
	return nil
}

func postgresPartitionedTableKey(database *gorm.DB, tableName string) (partitionKey string, exists bool, err error) {
	err = database.Raw(`
		SELECT
			to_regclass(?) IS NOT NULL AS exists,
			COALESCE(pg_get_partkeydef(to_regclass(?)), '') AS partition_key
	`, tableName, tableName).Row().Scan(&exists, &partitionKey)
	return partitionKey, exists, err
}

func migrateRegularTableToHashPartitions(database *gorm.DB, table hashPartitionedTable) error {
	legacyName := fmt.Sprintf("%s_legacy_%s", table.name, time.Now().UTC().Format("20060102150405"))
	if err := database.Exec(fmt.Sprintf(
		"ALTER TABLE %s RENAME TO %s",
		quotePostgresIdentifier(table.name),
		quotePostgresIdentifier(legacyName),
	)).Error; err != nil {
		return err
	}
	if err := renameLegacySerialSequence(database, table.name, legacyName, "id"); err != nil {
		return err
	}

	if err := database.Exec(table.createSQL).Error; err != nil {
		return err
	}
	for partitionIndex := range table.partitionCount {
		if err := database.Exec(createHashPartitionSQL(table.name, partitionIndex, table.partitionCount)).Error; err != nil {
			return err
		}
	}
	if err := copyLegacyRowsIntoHashPartitionedTable(database, table, legacyName); err != nil {
		return err
	}
	if err := syncSerialSequenceToMaxID(database, table.name, "id"); err != nil {
		return err
	}
	return database.Exec(fmt.Sprintf("DROP TABLE %s", quotePostgresIdentifier(legacyName))).Error
}

func copyLegacyRowsIntoHashPartitionedTable(database *gorm.DB, table hashPartitionedTable, legacyName string) error {
	columnList := quotedColumnList(table.columns)
	return database.Exec(fmt.Sprintf(
		"INSERT INTO %s (%s) SELECT %s FROM %s",
		quotePostgresIdentifier(table.name),
		columnList,
		columnList,
		quotePostgresIdentifier(legacyName),
	)).Error
}

func renameLegacySerialSequence(database *gorm.DB, tableName string, legacyName string, columnName string) error {
	return database.Exec(renameLegacySerialSequenceSQL(tableName, legacyName, columnName)).Error
}

func renameLegacySerialSequenceSQL(tableName string, legacyName string, columnName string) string {
	sequenceName := fmt.Sprintf("%s_%s_seq", tableName, columnName)
	legacySequenceName := fmt.Sprintf("%s_%s_seq", legacyName, columnName)
	return fmt.Sprintf(`
DO $$
DECLARE
	legacy_sequence text;
BEGIN
	SELECT pg_get_serial_sequence(%s, %s) INTO legacy_sequence;
	IF legacy_sequence IS NOT NULL
		AND to_regclass(legacy_sequence) = to_regclass(%s)
	THEN
		EXECUTE format('ALTER SEQUENCE %%s RENAME TO %%I', legacy_sequence, %s);
	END IF;
END $$`,
		quotePostgresStringLiteral(legacyName),
		quotePostgresStringLiteral(columnName),
		quotePostgresStringLiteral(sequenceName),
		quotePostgresStringLiteral(legacySequenceName),
	)
}

func syncSerialSequenceToMaxID(database *gorm.DB, tableName string, columnName string) error {
	return database.Exec(fmt.Sprintf(
		`SELECT setval(
			pg_get_serial_sequence('%s', '%s'),
			COALESCE((SELECT MAX(%s) FROM %s), 1),
			EXISTS (SELECT 1 FROM %s)
		)`,
		strings.ReplaceAll(tableName, "'", "''"),
		strings.ReplaceAll(columnName, "'", "''"),
		quotePostgresIdentifier(columnName),
		quotePostgresIdentifier(tableName),
		quotePostgresIdentifier(tableName),
	)).Error
}

func createHashPartitionSQL(tableName string, partitionIndex int, partitionCount int) string {
	partitionName := fmt.Sprintf("%s_p%02d", tableName, partitionIndex)
	return fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES WITH (MODULUS %d, REMAINDER %d)",
		quotePostgresIdentifier(partitionName),
		quotePostgresIdentifier(tableName),
		partitionCount,
		partitionIndex,
	)
}

func quotePostgresStringLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

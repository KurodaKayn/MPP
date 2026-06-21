package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	monthlyPartitionPastMonths   = 12
	monthlyPartitionFutureMonths = 3
)

type monthlyPartitionedTable struct {
	name      string
	createSQL string
	columns   []string
}

var monthlyEventPartitionedTables = []monthlyPartitionedTable{
	{
		name: "publish_events",
		createSQL: `
			CREATE TABLE IF NOT EXISTS publish_events (
				id uuid NOT NULL,
				publication_id uuid NOT NULL,
				project_id uuid NOT NULL,
				user_id uuid NOT NULL,
				platform text NOT NULL,
				job_id uuid NOT NULL,
				idempotency_key text NOT NULL,
				event_type text NOT NULL,
				status text NOT NULL,
				message text,
				remote_id text,
				publish_url text,
				error_message text,
				metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
				created_at timestamptz NOT NULL,
				CONSTRAINT pk_publish_events_partitioned PRIMARY KEY (id, created_at)
			) PARTITION BY RANGE (created_at)
		`,
		columns: []string{
			"id",
			"publication_id",
			"project_id",
			"user_id",
			"platform",
			"job_id",
			"idempotency_key",
			"event_type",
			"status",
			"message",
			"remote_id",
			"publish_url",
			"error_message",
			"metadata",
			"created_at",
		},
	},
	{
		name: "extension_execution_events",
		createSQL: `
			CREATE TABLE IF NOT EXISTS extension_execution_events (
				id uuid NOT NULL,
				callback_token_id uuid NOT NULL,
				execution_id text NOT NULL,
				project_id uuid NOT NULL,
				user_id uuid NOT NULL,
				event_id text NOT NULL,
				platform text NOT NULL,
				status text NOT NULL,
				message text,
				remote_id text,
				publish_url text,
				error_message text,
				metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
				created_at timestamptz NOT NULL,
				CONSTRAINT pk_extension_execution_events_partitioned PRIMARY KEY (id, created_at)
			) PARTITION BY RANGE (created_at)
		`,
		columns: []string{
			"id",
			"callback_token_id",
			"execution_id",
			"project_id",
			"user_id",
			"event_id",
			"platform",
			"status",
			"message",
			"remote_id",
			"publish_url",
			"error_message",
			"metadata",
			"created_at",
		},
	},
	{
		name: "project_activities",
		createSQL: `
			CREATE TABLE IF NOT EXISTS project_activities (
				id uuid NOT NULL,
				project_id uuid NOT NULL,
				actor_user_id uuid NOT NULL,
				target_user_id uuid,
				event_type text NOT NULL,
				metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
				created_at timestamptz NOT NULL,
				CONSTRAINT pk_project_activities_partitioned PRIMARY KEY (id, created_at)
			) PARTITION BY RANGE (created_at)
		`,
		columns: []string{
			"id",
			"project_id",
			"actor_user_id",
			"target_user_id",
			"event_type",
			"metadata",
			"created_at",
		},
	},
	{
		name: "workspace_activities",
		createSQL: `
			CREATE TABLE IF NOT EXISTS workspace_activities (
				id uuid NOT NULL,
				workspace_id uuid NOT NULL,
				actor_user_id uuid NOT NULL,
				target_user_id uuid,
				event_type text NOT NULL,
				metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
				created_at timestamptz NOT NULL,
				CONSTRAINT pk_workspace_activities_partitioned PRIMARY KEY (id, created_at)
			) PARTITION BY RANGE (created_at)
		`,
		columns: []string{
			"id",
			"workspace_id",
			"actor_user_id",
			"target_user_id",
			"event_type",
			"metadata",
			"created_at",
		},
	},
}

func ensureMonthlyEventPartitions(database *gorm.DB, now time.Time) error {
	if database.Name() != "postgres" {
		return nil
	}
	for _, table := range monthlyEventPartitionedTables {
		if err := ensureMonthlyPartitionedTable(database, table, now); err != nil {
			return err
		}
	}
	return nil
}

func ensureMonthlyPartitionedTable(database *gorm.DB, table monthlyPartitionedTable, now time.Time) error {
	partitioned, exists, err := postgresPartitionedTableState(database, table.name)
	if err != nil {
		return err
	}
	if exists && !partitioned {
		if err := migrateRegularTableToMonthlyPartitions(database, table, now); err != nil {
			return err
		}
	} else if !exists {
		if err := database.Exec(table.createSQL).Error; err != nil {
			return err
		}
	}
	return ensureRollingMonthlyPartitions(database, table.name, now)
}

func postgresPartitionedTableState(database *gorm.DB, tableName string) (partitioned bool, exists bool, err error) {
	err = database.Raw(`
		SELECT
			to_regclass(?) IS NOT NULL AS exists,
			EXISTS (
				SELECT 1
				FROM pg_partitioned_table
				WHERE partrelid = to_regclass(?)
			) AS partitioned
	`, tableName, tableName).Row().Scan(&exists, &partitioned)
	return partitioned, exists, err
}

func migrateRegularTableToMonthlyPartitions(database *gorm.DB, table monthlyPartitionedTable, now time.Time) error {
	legacyName := fmt.Sprintf("%s_legacy_%s", table.name, now.UTC().Format("20060102150405"))
	if err := database.Exec(fmt.Sprintf(
		"ALTER TABLE %s RENAME TO %s",
		quotePostgresIdentifier(table.name),
		quotePostgresIdentifier(legacyName),
	)).Error; err != nil {
		return err
	}

	if err := database.Exec(table.createSQL).Error; err != nil {
		return err
	}
	if err := ensureRollingMonthlyPartitions(database, table.name, now); err != nil {
		return err
	}
	if err := ensureLegacyDataMonthlyPartitions(database, table.name, legacyName); err != nil {
		return err
	}
	if err := copyLegacyRowsIntoPartitionedTable(database, table, legacyName, now); err != nil {
		return err
	}
	return database.Exec(fmt.Sprintf("DROP TABLE %s", quotePostgresIdentifier(legacyName))).Error
}

func ensureRollingMonthlyPartitions(database *gorm.DB, tableName string, now time.Time) error {
	start := monthStartUTC(now).AddDate(0, -monthlyPartitionPastMonths, 0)
	for offset := 0; offset <= monthlyPartitionPastMonths+monthlyPartitionFutureMonths; offset++ {
		if err := ensureMonthlyPartition(database, tableName, start.AddDate(0, offset, 0)); err != nil {
			return err
		}
	}
	return nil
}

func ensureLegacyDataMonthlyPartitions(database *gorm.DB, tableName string, legacyName string) error {
	var minCreatedAt, maxCreatedAt sql.NullTime
	if err := database.Raw(fmt.Sprintf(
		"SELECT MIN(created_at), MAX(created_at) FROM %s",
		quotePostgresIdentifier(legacyName),
	)).Row().Scan(&minCreatedAt, &maxCreatedAt); err != nil {
		return err
	}
	if !minCreatedAt.Valid || !maxCreatedAt.Valid {
		return nil
	}

	start := monthStartUTC(minCreatedAt.Time)
	end := monthStartUTC(maxCreatedAt.Time)
	for partitionStart := start; !partitionStart.After(end); partitionStart = partitionStart.AddDate(0, 1, 0) {
		if err := ensureMonthlyPartition(database, tableName, partitionStart); err != nil {
			return err
		}
	}
	return nil
}

func ensureMonthlyPartition(database *gorm.DB, tableName string, partitionStart time.Time) error {
	partitionStart = monthStartUTC(partitionStart)
	partitionEnd := partitionStart.AddDate(0, 1, 0)
	return database.Exec(createMonthlyPartitionSQL(tableName, partitionStart, partitionEnd)).Error
}

func copyLegacyRowsIntoPartitionedTable(database *gorm.DB, table monthlyPartitionedTable, legacyName string, now time.Time) error {
	columnList := quotedColumnList(table.columns)
	selectList := legacySelectColumnList(table.columns, now)
	return database.Exec(fmt.Sprintf(
		"INSERT INTO %s (%s) SELECT %s FROM %s",
		quotePostgresIdentifier(table.name),
		columnList,
		selectList,
		quotePostgresIdentifier(legacyName),
	)).Error
}

func createMonthlyPartitionSQL(tableName string, partitionStart time.Time, partitionEnd time.Time) string {
	partitionName := fmt.Sprintf("%s_%s", tableName, partitionStart.UTC().Format("2006_01"))
	return fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM (%s) TO (%s)",
		quotePostgresIdentifier(partitionName),
		quotePostgresIdentifier(tableName),
		quotePostgresTimestampLiteral(partitionStart),
		quotePostgresTimestampLiteral(partitionEnd),
	)
}

func quotedColumnList(columns []string) string {
	quoted := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted = append(quoted, quotePostgresIdentifier(column))
	}
	return strings.Join(quoted, ", ")
}

func legacySelectColumnList(columns []string, now time.Time) string {
	selected := make([]string, 0, len(columns))
	for _, column := range columns {
		if column == "created_at" {
			selected = append(selected, fmt.Sprintf(
				"COALESCE(%s, %s) AS %s",
				quotePostgresIdentifier(column),
				quotePostgresTimestampLiteral(now),
				quotePostgresIdentifier(column),
			))
			continue
		}
		selected = append(selected, quotePostgresIdentifier(column))
	}
	return strings.Join(selected, ", ")
}

func monthStartUTC(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func quotePostgresIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func quotePostgresTimestampLiteral(value time.Time) string {
	return "TIMESTAMPTZ '" + value.UTC().Format("2006-01-02 15:04:05-07") + "'"
}

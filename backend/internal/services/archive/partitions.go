package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
)

type monthlyPartitionArchiveSpec struct {
	Table            string
	BeforeDrop       func(context.Context, *gorm.DB, coldMonthlyPartition) error
	CanDropPartition func(context.Context, *gorm.DB, coldMonthlyPartition) (bool, error)
}

type coldMonthlyPartition struct {
	ParentSchema string
	ParentName   string
	Schema       string
	Name         string
	Start        time.Time
	End          time.Time
}

type archivePartitionLine struct {
	SchemaVersion   int             `json:"schema_version"`
	Table           string          `json:"table"`
	Partition       string          `json:"partition"`
	ArchivedAt      time.Time       `json:"archived_at"`
	RetentionCutoff time.Time       `json:"retention_cutoff"`
	PartitionStart  time.Time       `json:"partition_start"`
	PartitionEnd    time.Time       `json:"partition_end"`
	Row             json.RawMessage `json:"row"`
}

var publishEventsPartitionSpec = monthlyPartitionArchiveSpec{
	Table: "publish_events",
}

var extensionExecutionEventsPartitionSpec = monthlyPartitionArchiveSpec{
	Table:      "extension_execution_events",
	BeforeDrop: deleteExtensionExecutionEventClaimsForPartition,
}

var projectActivitiesPartitionSpec = monthlyPartitionArchiveSpec{
	Table: "project_activities",
}

var workspaceActivitiesPartitionSpec = monthlyPartitionArchiveSpec{
	Table: "workspace_activities",
}

var remoteBrowserSessionsPartitionSpec = monthlyPartitionArchiveSpec{
	Table:            "remote_browser_sessions",
	CanDropPartition: remoteBrowserSessionPartitionIsTerminal,
}

func archivePartitionedModel[T any](
	ctx context.Context,
	db *gorm.DB,
	storage objectstorage.Client,
	config Config,
	spec monthlyPartitionArchiveSpec,
	retention time.Duration,
	now time.Time,
	scope func(*gorm.DB, time.Time) *gorm.DB,
	beforeDelete archiveBeforeDeleteHook[T],
) (TableResult, error) {
	cutoff := now.Add(-retention)
	result := TableResult{Table: spec.Table, Cutoff: cutoff}

	if db.Name() == "postgres" {
		partitionResult, err := archiveColdMonthlyPartitions[T](ctx, db, storage, config, spec, retention, now)
		if err != nil {
			return result, err
		}
		mergeTableResult(&result, partitionResult)
	}

	rowResult, err := archiveModelWithDeleteHook[T](ctx, db, storage, config, spec.Table, retention, now, scope, beforeDelete)
	if err != nil {
		return result, err
	}
	mergeTableResult(&result, rowResult)
	return result, nil
}

func archiveColdMonthlyPartitions[T any](
	ctx context.Context,
	db *gorm.DB,
	storage objectstorage.Client,
	config Config,
	spec monthlyPartitionArchiveSpec,
	retention time.Duration,
	now time.Time,
) (TableResult, error) {
	cutoff := now.Add(-retention)
	result := TableResult{Table: spec.Table, Cutoff: cutoff}
	partitions, err := listColdMonthlyPartitions(ctx, db, spec.Table, cutoff)
	if err != nil {
		return result, err
	}

	for _, partition := range partitions {
		if spec.CanDropPartition != nil {
			canDrop, err := spec.CanDropPartition(ctx, db, partition)
			if err != nil {
				return result, err
			}
			if !canDrop {
				continue
			}
		}

		partitionResult, err := archiveColdMonthlyPartition[T](ctx, db, storage, config, spec, cutoff, now, partition)
		if err != nil {
			return result, err
		}
		mergeTableResult(&result, partitionResult)
	}
	return result, nil
}

func listColdMonthlyPartitions(ctx context.Context, db *gorm.DB, table string, cutoff time.Time) ([]coldMonthlyPartition, error) {
	rows, err := db.WithContext(ctx).Raw(`
		SELECT
			parent_namespace.nspname AS parent_schema,
			parent.relname AS parent_name,
			child_namespace.nspname AS partition_schema,
			child.relname AS partition_name
		FROM pg_inherits inheritance
		JOIN pg_class parent ON parent.oid = inheritance.inhparent
		JOIN pg_namespace parent_namespace ON parent_namespace.oid = parent.relnamespace
		JOIN pg_class child ON child.oid = inheritance.inhrelid
		JOIN pg_namespace child_namespace ON child_namespace.oid = child.relnamespace
		WHERE parent.oid = to_regclass(?)
		ORDER BY child.relname ASC
	`, table).Rows()
	if err != nil {
		return nil, fmt.Errorf("list %s monthly partitions: %w", table, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var partitions []coldMonthlyPartition
	for rows.Next() {
		var partition coldMonthlyPartition
		if err := rows.Scan(&partition.ParentSchema, &partition.ParentName, &partition.Schema, &partition.Name); err != nil {
			return nil, fmt.Errorf("scan %s monthly partition: %w", table, err)
		}
		start, end, ok := monthlyPartitionBounds(partition.ParentName, partition.Name)
		if !ok || end.After(cutoff) {
			continue
		}
		partition.Start = start
		partition.End = end
		partitions = append(partitions, partition)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s monthly partitions: %w", table, err)
	}
	return partitions, nil
}

func archiveColdMonthlyPartition[T any](
	ctx context.Context,
	db *gorm.DB,
	storage objectstorage.Client,
	config Config,
	spec monthlyPartitionArchiveSpec,
	cutoff time.Time,
	now time.Time,
	partition coldMonthlyPartition,
) (TableResult, error) {
	result := TableResult{Table: spec.Table, Cutoff: cutoff}
	tempFile, err := os.CreateTemp("", fmt.Sprintf("%s-%s-*.jsonl", spec.Table, partition.Name))
	if err != nil {
		return result, fmt.Errorf("create %s partition %s temp archive file: %w", spec.Table, partition.Name, err)
	}
	tempName := tempFile.Name()
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempName)
	}()

	rowsArchived, err := encodePartitionJSONLines[T](ctx, db, spec, cutoff, now, partition, tempFile)
	if err != nil {
		return result, err
	}
	key := ""
	if rowsArchived > 0 {
		if err := tempFile.Sync(); err != nil {
			return result, fmt.Errorf("sync %s partition %s temp archive file: %w", spec.Table, partition.Name, err)
		}
		if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
			return result, fmt.Errorf("rewind %s partition %s temp archive file: %w", spec.Table, partition.Name, err)
		}

		key = archivePartitionObjectKey(config.ObjectKeyPrefix, spec.Table, partition.Name, partition.Start, partition.End, now)
		if _, err := storage.PutObject(ctx, objectstorage.UploadObjectInput{
			Key:         key,
			ContentType: archiveContentType,
			Body:        tempFile,
		}); err != nil {
			return result, fmt.Errorf("upload %s partition %s archive object: %w", spec.Table, partition.Name, err)
		}
	}

	if err := dropArchivedMonthlyPartition(ctx, db, spec, partition); err != nil {
		return result, err
	}

	result.RowsArchived = rowsArchived
	if key != "" {
		result.ObjectKey = key
		result.ObjectKeys = []string{key}
	}
	result.PartitionsArchived = []string{partition.Name}
	return result, nil
}

func dropArchivedMonthlyPartition(ctx context.Context, db *gorm.DB, spec monthlyPartitionArchiveSpec, partition coldMonthlyPartition) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if spec.BeforeDrop != nil {
			if err := spec.BeforeDrop(ctx, tx, partition); err != nil {
				return err
			}
		}
		if err := tx.Exec(fmt.Sprintf(
			"ALTER TABLE %s DETACH PARTITION %s",
			partition.qualifiedParentName(),
			partition.qualifiedName(),
		)).Error; err != nil {
			return fmt.Errorf("detach %s partition %s: %w", spec.Table, partition.Name, err)
		}
		if err := tx.Exec(fmt.Sprintf(
			"DROP TABLE %s",
			partition.qualifiedName(),
		)).Error; err != nil {
			return fmt.Errorf("drop %s partition %s: %w", spec.Table, partition.Name, err)
		}
		return nil
	})
}

func encodePartitionJSONLines[T any](
	ctx context.Context,
	db *gorm.DB,
	spec monthlyPartitionArchiveSpec,
	cutoff time.Time,
	archivedAt time.Time,
	partition coldMonthlyPartition,
	writer io.Writer,
) (int, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(new(T)); err != nil {
		return 0, fmt.Errorf("parse %s partition %s row schema: %w", spec.Table, partition.Name, err)
	}
	columns := make([]string, 0, len(stmt.Schema.Fields))
	for _, field := range stmt.Schema.Fields {
		if field.DBName == "" {
			continue
		}
		columns = append(columns, field.DBName)
	}
	query := fmt.Sprintf(
		"SELECT %s FROM %s ORDER BY %s ASC, %s ASC",
		archiveQuotedColumnList(columns),
		partition.qualifiedName(),
		quoteArchivePostgresIdentifier("created_at"),
		quoteArchivePostgresIdentifier("id"),
	)
	rows, err := db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return 0, fmt.Errorf("query %s partition %s rows: %w", spec.Table, partition.Name, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	encoder := json.NewEncoder(writer)
	rowCount := 0
	for rows.Next() {
		var record T
		if err := db.WithContext(ctx).ScanRows(rows, &record); err != nil {
			return 0, fmt.Errorf("scan %s partition %s row: %w", spec.Table, partition.Name, err)
		}
		rowJSON, err := archivePartitionRowJSON(record, stmt.Schema.Fields)
		if err != nil {
			return 0, fmt.Errorf("marshal %s partition %s row: %w", spec.Table, partition.Name, err)
		}
		if err := encoder.Encode(archivePartitionLine{
			SchemaVersion:   1,
			Table:           spec.Table,
			Partition:       partition.Name,
			ArchivedAt:      archivedAt,
			RetentionCutoff: cutoff,
			PartitionStart:  partition.Start,
			PartitionEnd:    partition.End,
			Row:             json.RawMessage(rowJSON),
		}); err != nil {
			return 0, fmt.Errorf("encode %s partition %s rows: %w", spec.Table, partition.Name, err)
		}
		rowCount++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate %s partition %s rows: %w", spec.Table, partition.Name, err)
	}
	return rowCount, nil
}

func archivePartitionRowJSON(record any, fields []*schema.Field) ([]byte, error) {
	value := reflect.ValueOf(record)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, fmt.Errorf("archive record is nil")
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return nil, fmt.Errorf("archive record must be a struct, got %s", value.Kind())
	}

	payload := make(map[string]any, len(fields))
	for _, field := range fields {
		if field.DBName == "" {
			continue
		}
		fieldValue := value.FieldByName(field.Name)
		if !fieldValue.IsValid() {
			return nil, fmt.Errorf("archive record is missing field %q", field.Name)
		}
		payload[field.Name] = fieldValue.Interface()
	}
	return json.Marshal(payload)
}

func archiveQuotedColumnList(columns []string) string {
	quoted := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted = append(quoted, quoteArchivePostgresIdentifier(column))
	}
	return strings.Join(quoted, ", ")
}

func deleteExtensionExecutionEventClaimsForPartition(ctx context.Context, tx *gorm.DB, partition coldMonthlyPartition) error {
	if !tx.Migrator().HasTable(&models.ExtensionExecutionEventClaim{}) {
		return nil
	}
	return tx.WithContext(ctx).Exec(fmt.Sprintf(`
		DELETE FROM extension_execution_event_claims
		WHERE record_id IN (SELECT id FROM %s)
	`, partition.qualifiedName())).Error
}

func remoteBrowserSessionPartitionIsTerminal(ctx context.Context, db *gorm.DB, partition coldMonthlyPartition) (bool, error) {
	var hasNonTerminal bool
	err := db.WithContext(ctx).Raw(fmt.Sprintf(`
		SELECT EXISTS (
			SELECT 1
			FROM %s
			WHERE status NOT IN (?, ?, ?)
		)
	`, partition.qualifiedName()),
		models.BrowserSessionStatusConnected,
		models.BrowserSessionStatusExpired,
		models.BrowserSessionStatusFailed,
	).Scan(&hasNonTerminal).Error
	if err != nil {
		return false, fmt.Errorf("check remote browser session partition %s terminal status: %w", partition.Name, err)
	}
	return !hasNonTerminal, nil
}

func monthlyPartitionBounds(parentTable string, partitionName string) (time.Time, time.Time, bool) {
	suffix := strings.TrimPrefix(partitionName, parentTable+"_")
	if suffix == partitionName || suffix == "default" {
		return time.Time{}, time.Time{}, false
	}
	start, err := time.Parse("2006_01", suffix)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	start = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0), true
}

func archivePartitionObjectKey(prefix string, table string, partitionName string, partitionStart time.Time, partitionEnd time.Time, archivedAt time.Time) string {
	normalizedPrefix := strings.Trim(strings.TrimSpace(prefix), "/")
	if normalizedPrefix == "" {
		normalizedPrefix = defaultArchiveObjectPrefix
	}
	return fmt.Sprintf(
		"%s/%s/partitions/partition_start=%s/partition_end=%s/%s-%d.jsonl",
		normalizedPrefix,
		table,
		partitionStart.UTC().Format("2006-01-02"),
		partitionEnd.UTC().Format("2006-01-02"),
		partitionName,
		archivedAt.UTC().UnixNano(),
	)
}

func quoteArchivePostgresIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func quoteArchivePostgresQualifiedIdentifier(schema string, name string) string {
	if strings.TrimSpace(schema) == "" {
		return quoteArchivePostgresIdentifier(name)
	}
	return quoteArchivePostgresIdentifier(schema) + "." + quoteArchivePostgresIdentifier(name)
}

func (p coldMonthlyPartition) qualifiedName() string {
	return quoteArchivePostgresQualifiedIdentifier(p.Schema, p.Name)
}

func (p coldMonthlyPartition) qualifiedParentName() string {
	return quoteArchivePostgresQualifiedIdentifier(p.ParentSchema, p.ParentName)
}

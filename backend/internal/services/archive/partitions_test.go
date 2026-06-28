package archive

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
)

func TestMonthlyPartitionBoundsParsesManagedPartitionName(t *testing.T) {
	start, end, ok := monthlyPartitionBounds("publish_events", "publish_events_2026_01")

	if !ok {
		t.Fatalf("expected managed monthly partition to parse")
	}
	if !start.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected partition start %s", start)
	}
	if !end.Equal(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected partition end %s", end)
	}
}

func TestMonthlyPartitionBoundsRejectsDefaultOrUnknownNames(t *testing.T) {
	for _, partitionName := range []string{
		"publish_events_default",
		"publish_events_legacy_20260101000000",
		"extension_execution_events_2026_01",
	} {
		if _, _, ok := monthlyPartitionBounds("publish_events", partitionName); ok {
			t.Fatalf("expected %q to be ignored", partitionName)
		}
	}
}

func TestArchivePartitionObjectKeyUsesPartitionRange(t *testing.T) {
	key := archivePartitionObjectKey(
		" /cold/database/ ",
		"publish_events",
		"publish_events_2026_01",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		time.Unix(123, 456).UTC(),
	)

	if !strings.HasPrefix(key, "cold/database/publish_events/partitions/partition_start=2026-01-01/partition_end=2026-02-01/") {
		t.Fatalf("unexpected archive key prefix %q", key)
	}
	if !strings.HasSuffix(key, "/publish_events_2026_01-123000000456.jsonl") {
		t.Fatalf("unexpected archive key suffix %q", key)
	}
}

func TestArchiveQuotedColumnListQuotesIdentifiers(t *testing.T) {
	columns := archiveQuotedColumnList([]string{"id", `strange"column`})

	if columns != `"id", "strange""column"` {
		t.Fatalf("unexpected column list %q", columns)
	}
}

func TestEncodePartitionJSONLinesPreservesStructFieldNames(t *testing.T) {
	db := setupArchiveTestDB(t)
	if err := db.Exec(`CREATE TABLE publish_events_2026_01 (
		id TEXT PRIMARY KEY,
		publication_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		project_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		job_id TEXT NOT NULL,
		idempotency_key TEXT NOT NULL,
		event_type TEXT NOT NULL,
		status TEXT NOT NULL,
		message TEXT,
		remote_id TEXT,
		publish_url TEXT,
		error_message TEXT,
		metadata TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create partition table: %v", err)
	}

	archivedAt := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	partitionStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	event := models.PublishEvent{
		ID:             uuid.New(),
		PublicationID:  uuid.New(),
		WorkspaceID:    uuid.New(),
		ProjectID:      uuid.New(),
		UserID:         uuid.New(),
		Platform:       "wechat",
		JobID:          uuid.New(),
		IdempotencyKey: "cold-a",
		EventType:      "publish.completed",
		Status:         models.PublicationStatusSucceeded,
		Metadata:       datatypes.JSON(`{"source":"test"}`),
		CreatedAt:      partitionStart.Add(time.Hour),
	}
	if err := db.Table("publish_events_2026_01").Create(&event).Error; err != nil {
		t.Fatalf("insert partition row: %v", err)
	}

	var body bytes.Buffer
	rowsArchived, err := encodePartitionJSONLines[models.PublishEvent](
		context.Background(),
		db,
		monthlyPartitionArchiveSpec{Table: "publish_events"},
		archivedAt.Add(-180*24*time.Hour),
		archivedAt,
		coldMonthlyPartition{
			Name:  "publish_events_2026_01",
			Start: partitionStart,
			End:   partitionStart.AddDate(0, 1, 0),
		},
		&body,
	)
	if err != nil {
		t.Fatalf("encode partition JSONL: %v", err)
	}
	if rowsArchived != 1 {
		t.Fatalf("expected one archived row, got %d", rowsArchived)
	}

	lines := jsonLines(t, body.String())
	if len(lines) != 1 {
		t.Fatalf("expected one JSONL line, got %d", len(lines))
	}
	row := lines[0]["row"].(map[string]any)
	if _, ok := row["ID"]; !ok {
		t.Fatalf("expected exported row to use Go field names, got %#v", row)
	}
	if _, ok := row["id"]; ok {
		t.Fatalf("expected exported row to avoid snake_case database keys, got %#v", row)
	}
}

func TestArchiveColdMonthlyPartitionDropsEmptyPartitionWithoutUpload(t *testing.T) {
	state := &archivePartitionTestState{}
	db := openArchivePartitionTestDB(t, state)
	storage := &rejectPutPartitionStorage{}
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	partitionStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	partition := coldMonthlyPartition{
		ParentName: "publish_events",
		Name:       "publish_events_2026_01",
		Start:      partitionStart,
		End:        partitionStart.AddDate(0, 1, 0),
	}

	result, err := archiveColdMonthlyPartition[models.PublishEvent](
		context.Background(),
		db,
		storage,
		Config{ObjectKeyPrefix: "cold"},
		monthlyPartitionArchiveSpec{Table: "publish_events"},
		now.Add(-180*24*time.Hour),
		now,
		partition,
	)
	if err != nil {
		t.Fatalf("archive empty partition: %v", err)
	}

	if storage.putObjects != 0 {
		t.Fatalf("expected empty partition to skip upload, got %d uploads", storage.putObjects)
	}
	if result.RowsArchived != 0 {
		t.Fatalf("expected zero archived rows, got %d", result.RowsArchived)
	}
	if result.ObjectKey != "" || len(result.ObjectKeys) != 0 {
		t.Fatalf("expected no archive object for empty partition, got %q %#v", result.ObjectKey, result.ObjectKeys)
	}
	if len(result.PartitionsArchived) != 1 || result.PartitionsArchived[0] != partition.Name {
		t.Fatalf("expected partition to be marked archived, got %#v", result.PartitionsArchived)
	}

	expectedExecs := []string{
		`ALTER TABLE "publish_events" DETACH PARTITION "publish_events_2026_01"`,
		`DROP TABLE "publish_events_2026_01"`,
	}
	if len(state.execs) != len(expectedExecs) {
		t.Fatalf("expected detach and drop statements, got %#v", state.execs)
	}
	for i, expected := range expectedExecs {
		if state.execs[i] != expected {
			t.Fatalf("unexpected exec %d: got %q want %q", i, state.execs[i], expected)
		}
	}
}

type rejectPutPartitionStorage struct {
	objectstorage.Client
	putObjects int
}

func (s *rejectPutPartitionStorage) PutObject(context.Context, objectstorage.UploadObjectInput) (objectstorage.ObjectInfo, error) {
	s.putObjects++
	return objectstorage.ObjectInfo{}, fmt.Errorf("empty partition should not upload an archive object")
}

var registerArchivePartitionDriverOnce sync.Once
var archivePartitionDriverState *archivePartitionTestState

type archivePartitionTestState struct {
	queries []string
	execs   []string
}

type archivePartitionDriver struct{}

func (archivePartitionDriver) Open(_ string) (driver.Conn, error) {
	return &archivePartitionConn{state: archivePartitionDriverState}, nil
}

type archivePartitionConn struct {
	state *archivePartitionTestState
}

func (c *archivePartitionConn) Prepare(_ string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}

func (c *archivePartitionConn) Close() error {
	return nil
}

func (c *archivePartitionConn) Begin() (driver.Tx, error) {
	return archivePartitionTx{}, nil
}

func (c *archivePartitionConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.state.queries = append(c.state.queries, query)
	if strings.Contains(query, `FROM "publish_events_2026_01"`) {
		return archivePartitionRows{}, nil
	}
	return nil, fmt.Errorf("unexpected archive partition query: %s", query)
}

func (c *archivePartitionConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.state.execs = append(c.state.execs, query)
	if query == `ALTER TABLE "publish_events" DETACH PARTITION "publish_events_2026_01"` ||
		query == `DROP TABLE "publish_events_2026_01"` {
		return driver.RowsAffected(0), nil
	}
	return nil, fmt.Errorf("unexpected archive partition exec: %s", query)
}

type archivePartitionTx struct{}

func (archivePartitionTx) Commit() error {
	return nil
}

func (archivePartitionTx) Rollback() error {
	return nil
}

type archivePartitionRows struct{}

func (archivePartitionRows) Columns() []string {
	return []string{"id", "created_at"}
}

func (archivePartitionRows) Close() error {
	return nil
}

func (archivePartitionRows) Next(_ []driver.Value) error {
	return io.EOF
}

func openArchivePartitionTestDB(t *testing.T, state *archivePartitionTestState) *gorm.DB {
	t.Helper()
	registerArchivePartitionDriverOnce.Do(func() {
		sql.Register("archive-partition-test", archivePartitionDriver{})
	})
	archivePartitionDriverState = state
	sqlDB, err := sql.Open("archive-partition-test", "")
	if err != nil {
		t.Fatalf("open archive partition sql db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open archive partition gorm db: %v", err)
	}
	return db
}

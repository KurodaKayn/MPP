package archive

import (
	"bufio"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
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
	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage/fake"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestWorkerRunOnceArchivesColdEventsAndDeletesRows(t *testing.T) {
	db := setupArchiveTestDB(t)
	storage := fake.NewClient()
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	coldCreatedAt := now.Add(-181 * 24 * time.Hour)
	hotCreatedAt := now.Add(-10 * 24 * time.Hour)
	userID := uuid.New()
	projectID := uuid.New()
	publicationID := uuid.New()
	jobID := uuid.New()

	coldEvents := []models.PublishEvent{}
	for i := range 3 {
		coldEvents = append(coldEvents, models.PublishEvent{
			PublicationID:  publicationID,
			ProjectID:      projectID,
			UserID:         userID,
			Platform:       "wechat",
			JobID:          jobID,
			IdempotencyKey: "cold-" + string(rune('a'+i)),
			EventType:      "publish.completed",
			Status:         models.PublicationStatusSucceeded,
			Metadata:       datatypes.JSON(`{"source":"test"}`),
			CreatedAt:      coldCreatedAt.Add(time.Duration(i) * time.Second),
		})
	}
	hotEvent := models.PublishEvent{
		PublicationID:  publicationID,
		ProjectID:      projectID,
		UserID:         userID,
		Platform:       "wechat",
		JobID:          uuid.New(),
		IdempotencyKey: "hot",
		EventType:      "publish.completed",
		Status:         models.PublicationStatusSucceeded,
		Metadata:       datatypes.JSON(`{}`),
		CreatedAt:      hotCreatedAt,
	}
	for i := range coldEvents {
		if err := db.Create(&coldEvents[i]).Error; err != nil {
			t.Fatalf("create cold publish event: %v", err)
		}
	}
	if err := db.Create(&hotEvent).Error; err != nil {
		t.Fatalf("create hot publish event: %v", err)
	}

	worker := NewWorker(db, storage, Config{
		Enabled:                          true,
		Interval:                         time.Hour,
		BatchSize:                        2,
		ObjectKeyPrefix:                  "test-archive",
		PublishEventRetention:            180 * 24 * time.Hour,
		ExtensionExecutionEventRetention: 180 * 24 * time.Hour,
		ProjectActivityRetention:         365 * 24 * time.Hour,
		WorkspaceActivityRetention:       365 * 24 * time.Hour,
		BrowserSessionHistoryRetention:   90 * 24 * time.Hour,
	})
	result, err := worker.RunOnce(context.Background(), now)
	if err != nil {
		t.Fatalf("run archive worker: %v", err)
	}

	publishResult := tableResult(result, "publish_events")
	if publishResult.RowsArchived != 3 {
		t.Fatalf("expected three archived publish events, got %d", publishResult.RowsArchived)
	}
	if len(publishResult.ObjectKeys) != 2 {
		t.Fatalf("expected two archive objects, got %#v", publishResult.ObjectKeys)
	}
	if !strings.HasPrefix(publishResult.ObjectKey, "test-archive/publish_events/cutoff_date=2025-12-13/") {
		t.Fatalf("unexpected archive object key %q", publishResult.ObjectKey)
	}

	var remaining []models.PublishEvent
	if err := db.Order("id ASC").Find(&remaining).Error; err != nil {
		t.Fatalf("query remaining publish events: %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != hotEvent.ID {
		t.Fatalf("expected only hot event to remain, got %#v", remaining)
	}

	archivedIDs := map[string]bool{}
	totalLines := 0
	for _, objectKey := range publishResult.ObjectKeys {
		body := readObject(t, storage, objectKey)
		lines := jsonLines(t, body)
		totalLines += len(lines)
		for _, line := range lines {
			if line["table"] != "publish_events" {
				t.Fatalf("expected table metadata, got %#v", line["table"])
			}
			row := line["row"].(map[string]any)
			archivedIDs[row["ID"].(string)] = true
		}
	}
	if totalLines != len(coldEvents) {
		t.Fatalf("expected %d archived JSONL rows, got %d", len(coldEvents), totalLines)
	}
	for _, coldEvent := range coldEvents {
		if !archivedIDs[coldEvent.ID.String()] {
			t.Fatalf("expected archived row ID %s, got %#v", coldEvent.ID, archivedIDs)
		}
	}
}

func TestWorkerRunOnceArchivesOnlyTerminalColdBrowserSessions(t *testing.T) {
	db := setupArchiveTestDB(t)
	storage := fake.NewClient()
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	coldCreatedAt := now.Add(-91 * 24 * time.Hour)
	userID := uuid.New()

	terminal := models.RemoteBrowserSession{
		UserID:                userID,
		WorkspaceID:           nil,
		Platform:              "douyin",
		Status:                models.BrowserSessionStatusExpired,
		WorkerSessionRef:      "worker-old",
		ConnectTokenHash:      "",
		ConnectTokenExpiresAt: coldCreatedAt.Add(time.Hour),
		CreatedAt:             coldCreatedAt,
		ExpiresAt:             coldCreatedAt.Add(time.Hour),
	}
	active := models.RemoteBrowserSession{
		UserID:                userID,
		WorkspaceID:           nil,
		Platform:              "zhihu",
		Status:                models.BrowserSessionStatusReady,
		WorkerSessionRef:      "worker-active",
		ConnectTokenHash:      "token-hash",
		ConnectTokenExpiresAt: coldCreatedAt.Add(time.Hour),
		CreatedAt:             coldCreatedAt,
		ExpiresAt:             coldCreatedAt.Add(time.Hour),
	}
	if err := db.Create(&terminal).Error; err != nil {
		t.Fatalf("create terminal session: %v", err)
	}
	if err := db.Create(&active).Error; err != nil {
		t.Fatalf("create active session: %v", err)
	}

	worker := NewWorker(db, storage, Config{
		Enabled:                          true,
		Interval:                         time.Hour,
		BatchSize:                        10,
		ObjectKeyPrefix:                  "test-archive",
		PublishEventRetention:            180 * 24 * time.Hour,
		ExtensionExecutionEventRetention: 180 * 24 * time.Hour,
		ProjectActivityRetention:         365 * 24 * time.Hour,
		WorkspaceActivityRetention:       365 * 24 * time.Hour,
		BrowserSessionHistoryRetention:   90 * 24 * time.Hour,
	})
	result, err := worker.RunOnce(context.Background(), now)
	if err != nil {
		t.Fatalf("run archive worker: %v", err)
	}

	sessionResult := tableResult(result, "remote_browser_sessions")
	if sessionResult.RowsArchived != 1 {
		t.Fatalf("expected one archived session, got %d", sessionResult.RowsArchived)
	}

	var remaining []models.RemoteBrowserSession
	if err := db.Find(&remaining).Error; err != nil {
		t.Fatalf("query remaining sessions: %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != active.ID {
		t.Fatalf("expected only active session to remain, got %#v", remaining)
	}
}

func TestRunOnceSkipsWhenPostgresArchiveLockIsHeld(t *testing.T) {
	state := &archiveLockTestState{tryLockResult: false}
	db := openArchiveLockTestDB(t, state)
	worker := NewWorker(db, fake.NewClient(), Config{
		Enabled:                          true,
		Interval:                         time.Hour,
		BatchSize:                        10,
		ObjectKeyPrefix:                  "test-archive",
		PublishEventRetention:            180 * 24 * time.Hour,
		ExtensionExecutionEventRetention: 180 * 24 * time.Hour,
		ProjectActivityRetention:         365 * 24 * time.Hour,
		WorkspaceActivityRetention:       365 * 24 * time.Hour,
		BrowserSessionHistoryRetention:   90 * 24 * time.Hour,
	})

	result, err := worker.RunOnce(context.Background(), time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("run archive worker: %v", err)
	}
	if len(result.Tables) != 0 {
		t.Fatalf("expected lock-held run to skip archive tables, got %#v", result.Tables)
	}
	if len(state.queries) != 1 || !strings.Contains(state.queries[0], "pg_try_advisory_lock") {
		t.Fatalf("expected only advisory lock query, got %#v", state.queries)
	}
}

func tableResult(result RunResult, table string) TableResult {
	for _, tableResult := range result.Tables {
		if tableResult.Table == table {
			return tableResult
		}
	}
	return TableResult{}
}

var registerArchiveLockDriverOnce sync.Once
var archiveLockDriverState *archiveLockTestState

type archiveLockTestState struct {
	tryLockResult bool
	queries       []string
}

type archiveLockDriver struct{}

func (archiveLockDriver) Open(_ string) (driver.Conn, error) {
	return &archiveLockConn{state: archiveLockDriverState}, nil
}

type archiveLockConn struct {
	state *archiveLockTestState
}

func (c *archiveLockConn) Prepare(_ string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}

func (c *archiveLockConn) Close() error {
	return nil
}

func (c *archiveLockConn) Begin() (driver.Tx, error) {
	return archiveLockTx{}, nil
}

func (c *archiveLockConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.state.queries = append(c.state.queries, query)
	if strings.Contains(query, "pg_try_advisory_lock") {
		return &archiveLockRows{value: c.state.tryLockResult}, nil
	}
	if strings.Contains(query, "pg_advisory_unlock") {
		return &archiveLockRows{value: true}, nil
	}
	return nil, driver.ErrSkip
}

type archiveLockTx struct{}

func (archiveLockTx) Commit() error {
	return nil
}

func (archiveLockTx) Rollback() error {
	return nil
}

type archiveLockRows struct {
	value bool
	read  bool
}

func (r *archiveLockRows) Columns() []string {
	return []string{"locked"}
}

func (r *archiveLockRows) Close() error {
	return nil
}

func (r *archiveLockRows) Next(dest []driver.Value) error {
	if r.read {
		return io.EOF
	}
	dest[0] = r.value
	r.read = true
	return nil
}

func openArchiveLockTestDB(t *testing.T, state *archiveLockTestState) *gorm.DB {
	t.Helper()
	registerArchiveLockDriverOnce.Do(func() {
		sql.Register("archive-lock-test", archiveLockDriver{})
	})
	archiveLockDriverState = state
	sqlDB, err := sql.Open("archive-lock-test", "")
	if err != nil {
		t.Fatalf("open archive lock sql db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open archive lock gorm db: %v", err)
	}
	return db
}

func setupArchiveTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testsupport.SetupTestDB()
	if err := db.Exec(`CREATE TABLE publish_events (
		id TEXT PRIMARY KEY,
		publication_id TEXT NOT NULL,
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
		t.Fatalf("create publish_events table: %v", err)
	}
	return db
}

func readObject(t *testing.T, storage objectstorage.Client, key string) string {
	t.Helper()
	body, _, err := storage.GetObject(context.Background(), key)
	if err != nil {
		t.Fatalf("read archive object: %v", err)
	}
	contents, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read archive object body: %v", err)
	}
	if err := body.Close(); err != nil {
		t.Fatalf("close archive object body: %v", err)
	}
	return string(contents)
}

func jsonLines(t *testing.T, body string) []map[string]any {
	t.Helper()
	var lines []map[string]any
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		var line map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatalf("decode archive JSONL: %v", err)
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan archive JSONL: %v", err)
	}
	return lines
}

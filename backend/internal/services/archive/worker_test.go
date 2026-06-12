package archive

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
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

	coldEvent := models.PublishEvent{
		PublicationID:  publicationID,
		ProjectID:      projectID,
		UserID:         userID,
		Platform:       "wechat",
		JobID:          jobID,
		IdempotencyKey: "cold",
		EventType:      "publish.completed",
		Status:         models.PublicationStatusSucceeded,
		Metadata:       datatypes.JSON(`{"source":"test"}`),
		CreatedAt:      coldCreatedAt,
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
	if err := db.Create(&coldEvent).Error; err != nil {
		t.Fatalf("create cold publish event: %v", err)
	}
	if err := db.Create(&hotEvent).Error; err != nil {
		t.Fatalf("create hot publish event: %v", err)
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

	publishResult := tableResult(result, "publish_events")
	if publishResult.RowsArchived != 1 {
		t.Fatalf("expected one archived publish event, got %d", publishResult.RowsArchived)
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

	body := readObject(t, storage, publishResult.ObjectKey)
	lines := jsonLines(t, body)
	if len(lines) != 1 {
		t.Fatalf("expected one archived JSONL row, got %d", len(lines))
	}
	if lines[0]["table"] != "publish_events" {
		t.Fatalf("expected table metadata, got %#v", lines[0]["table"])
	}
	row := lines[0]["row"].(map[string]any)
	if row["ID"] != coldEvent.ID.String() {
		t.Fatalf("expected archived row ID %s, got %#v", coldEvent.ID, row["ID"])
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

func tableResult(result RunResult, table string) TableResult {
	for _, tableResult := range result.Tables {
		if tableResult.Table == table {
			return tableResult
		}
	}
	return TableResult{}
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

package archive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
)

const archiveContentType = "application/x-ndjson"
const archiveWorkerAdvisoryLockKey int64 = 2026061201

type Worker struct {
	db      *gorm.DB
	storage objectstorage.Client
	config  Config
}

type RunResult struct {
	Tables []TableResult
}

type TableResult struct {
	Table        string
	RowsArchived int
	ObjectKey    string
	ObjectKeys   []string
	Cutoff       time.Time
}

type archiveLine[T any] struct {
	SchemaVersion   int       `json:"schema_version"`
	Table           string    `json:"table"`
	ArchivedAt      time.Time `json:"archived_at"`
	RetentionCutoff time.Time `json:"retention_cutoff"`
	Row             T         `json:"row"`
}

// NewWorker creates a cold-row archival worker.
func NewWorker(db *gorm.DB, storage objectstorage.Client, config Config) *Worker {
	return &Worker{db: db, storage: storage, config: config}
}

// Start runs archive batches periodically until the context is cancelled.
func (w *Worker) Start(ctx context.Context) {
	if w == nil || !w.config.Enabled || w.db == nil || w.storage == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(w.config.Interval)
		defer ticker.Stop()
		for {
			if _, err := w.RunOnce(ctx, time.Now().UTC()); err != nil {
				log.Printf("event archive worker failed: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// RunOnce archives eligible batches for each managed table.
func (w *Worker) RunOnce(ctx context.Context, now time.Time) (RunResult, error) {
	if w == nil || w.db == nil || w.storage == nil {
		return RunResult{}, nil
	}
	now = now.UTC()
	if w.db.Name() != "postgres" {
		return w.runArchiveTables(ctx, w.db, now)
	}

	var result RunResult
	err := w.db.WithContext(ctx).Connection(func(connection *gorm.DB) error {
		locked, err := tryArchiveWorkerLock(ctx, connection)
		if err != nil {
			return err
		}
		if !locked {
			return nil
		}

		tableResult, runErr := w.runArchiveTables(ctx, connection, now)
		unlockErr := releaseArchiveWorkerLock(context.Background(), connection)
		if runErr != nil {
			return runErr
		}
		if unlockErr != nil {
			return unlockErr
		}
		result = tableResult
		return nil
	})
	return result, err
}

func (w *Worker) runArchiveTables(ctx context.Context, db *gorm.DB, now time.Time) (RunResult, error) {
	result := RunResult{}

	tables := []func(context.Context, *gorm.DB, time.Time) (TableResult, error){
		w.archivePublishEvents,
		w.archiveExtensionExecutionEvents,
		w.archiveProjectActivities,
		w.archiveWorkspaceActivities,
		w.archiveRemoteBrowserSessions,
	}
	for _, archiveTable := range tables {
		tableResult, err := archiveTable(ctx, db, now)
		if err != nil {
			return result, err
		}
		result.Tables = append(result.Tables, tableResult)
	}
	return result, nil
}

func (w *Worker) archivePublishEvents(ctx context.Context, db *gorm.DB, now time.Time) (TableResult, error) {
	return archiveModel[models.PublishEvent](ctx, db, w.storage, w.config, "publish_events", w.config.PublishEventRetention, now, func(query *gorm.DB, cutoff time.Time) *gorm.DB {
		return query.Where("created_at < ?", cutoff)
	})
}

func (w *Worker) archiveExtensionExecutionEvents(ctx context.Context, db *gorm.DB, now time.Time) (TableResult, error) {
	return archiveModel[models.ExtensionExecutionEvent](ctx, db, w.storage, w.config, "extension_execution_events", w.config.ExtensionExecutionEventRetention, now, func(query *gorm.DB, cutoff time.Time) *gorm.DB {
		return query.Where("created_at < ?", cutoff)
	})
}

func (w *Worker) archiveProjectActivities(ctx context.Context, db *gorm.DB, now time.Time) (TableResult, error) {
	return archiveModel[models.ProjectActivity](ctx, db, w.storage, w.config, "project_activities", w.config.ProjectActivityRetention, now, func(query *gorm.DB, cutoff time.Time) *gorm.DB {
		return query.Where("created_at < ?", cutoff)
	})
}

func (w *Worker) archiveWorkspaceActivities(ctx context.Context, db *gorm.DB, now time.Time) (TableResult, error) {
	return archiveModel[models.WorkspaceActivity](ctx, db, w.storage, w.config, "workspace_activities", w.config.WorkspaceActivityRetention, now, func(query *gorm.DB, cutoff time.Time) *gorm.DB {
		return query.Where("created_at < ?", cutoff)
	})
}

func (w *Worker) archiveRemoteBrowserSessions(ctx context.Context, db *gorm.DB, now time.Time) (TableResult, error) {
	terminalStatuses := []string{
		models.BrowserSessionStatusConnected,
		models.BrowserSessionStatusExpired,
		models.BrowserSessionStatusFailed,
	}
	return archiveModel[models.RemoteBrowserSession](ctx, db, w.storage, w.config, "remote_browser_sessions", w.config.BrowserSessionHistoryRetention, now, func(query *gorm.DB, cutoff time.Time) *gorm.DB {
		return query.Where("created_at < ? AND status IN ?", cutoff, terminalStatuses)
	})
}

func archiveModel[T any](
	ctx context.Context,
	db *gorm.DB,
	storage objectstorage.Client,
	config Config,
	table string,
	retention time.Duration,
	now time.Time,
	scope func(*gorm.DB, time.Time) *gorm.DB,
) (TableResult, error) {
	cutoff := now.Add(-retention)
	result := TableResult{Table: table, Cutoff: cutoff}

	for batchNumber := 0; ; batchNumber++ {
		batchResult, err := archiveModelBatch[T](ctx, db, storage, config, table, cutoff, now, batchNumber, scope)
		if err != nil {
			return result, err
		}
		if batchResult.RowsArchived == 0 {
			return result, nil
		}

		result.RowsArchived += batchResult.RowsArchived
		result.ObjectKeys = append(result.ObjectKeys, batchResult.ObjectKey)
		if result.ObjectKey == "" {
			result.ObjectKey = batchResult.ObjectKey
		}
	}
}

func archiveModelBatch[T any](
	ctx context.Context,
	db *gorm.DB,
	storage objectstorage.Client,
	config Config,
	table string,
	cutoff time.Time,
	now time.Time,
	batchNumber int,
	scope func(*gorm.DB, time.Time) *gorm.DB,
) (TableResult, error) {
	result := TableResult{Table: table, Cutoff: cutoff}
	var records []T
	query := db.WithContext(ctx).Order("created_at ASC, id ASC").Limit(config.BatchSize)
	if scope != nil {
		query = scope(query, cutoff)
	}
	if err := query.Find(&records).Error; err != nil {
		return result, fmt.Errorf("query %s archive rows: %w", table, err)
	}
	if len(records) == 0 {
		return result, nil
	}

	body, err := encodeJSONLines(table, cutoff, now, records)
	if err != nil {
		return result, err
	}
	key := archiveObjectKey(config.ObjectKeyPrefix, table, cutoff, now, batchNumber)
	if _, err := storage.PutObject(ctx, objectstorage.UploadObjectInput{
		Key:         key,
		ContentType: archiveContentType,
		Body:        bytes.NewReader(body),
	}); err != nil {
		return result, fmt.Errorf("upload %s archive object: %w", table, err)
	}
	deleteResult := db.WithContext(ctx).Delete(&records)
	if deleteResult.Error != nil {
		return result, fmt.Errorf("delete archived %s rows: %w", table, deleteResult.Error)
	}
	if deleteResult.RowsAffected != int64(len(records)) {
		return result, fmt.Errorf("delete archived %s rows: expected %d, deleted %d", table, len(records), deleteResult.RowsAffected)
	}

	result.RowsArchived = len(records)
	result.ObjectKey = key
	result.ObjectKeys = []string{key}
	return result, nil
}

func tryArchiveWorkerLock(ctx context.Context, db *gorm.DB) (bool, error) {
	var locked bool
	if err := db.WithContext(ctx).Raw("SELECT pg_try_advisory_lock(?)", archiveWorkerAdvisoryLockKey).Scan(&locked).Error; err != nil {
		return false, fmt.Errorf("acquire archive worker lock: %w", err)
	}
	return locked, nil
}

func releaseArchiveWorkerLock(ctx context.Context, db *gorm.DB) error {
	var released bool
	if err := db.WithContext(ctx).Raw("SELECT pg_advisory_unlock(?)", archiveWorkerAdvisoryLockKey).Scan(&released).Error; err != nil {
		return fmt.Errorf("release archive worker lock: %w", err)
	}
	if !released {
		return fmt.Errorf("release archive worker lock: lock was not held")
	}
	return nil
}

func encodeJSONLines[T any](table string, cutoff time.Time, archivedAt time.Time, records []T) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	for _, record := range records {
		if err := encoder.Encode(archiveLine[T]{
			SchemaVersion:   1,
			Table:           table,
			ArchivedAt:      archivedAt,
			RetentionCutoff: cutoff,
			Row:             record,
		}); err != nil {
			return nil, fmt.Errorf("encode %s archive rows: %w", table, err)
		}
	}
	return buffer.Bytes(), nil
}

func archiveObjectKey(prefix string, table string, cutoff time.Time, archivedAt time.Time, batchNumber int) string {
	normalizedPrefix := strings.Trim(strings.TrimSpace(prefix), "/")
	if normalizedPrefix == "" {
		normalizedPrefix = defaultArchiveObjectPrefix
	}
	return fmt.Sprintf(
		"%s/%s/cutoff_date=%s/batch-%d-%04d.jsonl",
		normalizedPrefix,
		table,
		cutoff.UTC().Format("2006-01-02"),
		archivedAt.UTC().UnixNano(),
		batchNumber,
	)
}

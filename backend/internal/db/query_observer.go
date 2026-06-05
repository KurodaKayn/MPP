package db

import (
	"context"
	"errors"
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	queryObserverPluginName   = "mpp_query_observer"
	queryObserverStartedAtKey = "mpp_query_observer_started_at"
)

// QueryObservation is the sanitized query fact emitted by the GORM callback layer.
type QueryObservation struct {
	Operation    string
	Table        string
	SQL          string
	QueryHash    string
	Duration     time.Duration
	RowsAffected int64
	Err          error
}

type QueryObserver interface {
	ObserveQuery(context.Context, QueryObservation)
}

func InstallQueryObserver(database *gorm.DB, observer QueryObserver) error {
	if database == nil || observer == nil {
		return nil
	}

	if err := database.Use(queryObserverPlugin{observer: observer}); err != nil {
		if errors.Is(err, gorm.ErrRegistered) {
			return nil
		}
		return err
	}
	return nil
}

type queryObserverPlugin struct {
	observer QueryObserver
}

func (p queryObserverPlugin) Name() string {
	return queryObserverPluginName
}

func (p queryObserverPlugin) Initialize(database *gorm.DB) error {
	if err := database.Callback().Create().Before("gorm:create").Register(
		"mpp:query_observer:create:start",
		startQueryObservation,
	); err != nil {
		return err
	}
	if err := database.Callback().Create().After("gorm:after_create").Register(
		"mpp:query_observer:create:finish",
		finishQueryObservation(p.observer, "create"),
	); err != nil {
		return err
	}

	if err := database.Callback().Query().Before("gorm:query").Register(
		"mpp:query_observer:query:start",
		startQueryObservation,
	); err != nil {
		return err
	}
	if err := database.Callback().Query().After("gorm:after_query").Register(
		"mpp:query_observer:query:finish",
		finishQueryObservation(p.observer, "query"),
	); err != nil {
		return err
	}

	if err := database.Callback().Update().Before("gorm:update").Register(
		"mpp:query_observer:update:start",
		startQueryObservation,
	); err != nil {
		return err
	}
	if err := database.Callback().Update().After("gorm:after_update").Register(
		"mpp:query_observer:update:finish",
		finishQueryObservation(p.observer, "update"),
	); err != nil {
		return err
	}

	if err := database.Callback().Delete().Before("gorm:delete").Register(
		"mpp:query_observer:delete:start",
		startQueryObservation,
	); err != nil {
		return err
	}
	if err := database.Callback().Delete().After("gorm:after_delete").Register(
		"mpp:query_observer:delete:finish",
		finishQueryObservation(p.observer, "delete"),
	); err != nil {
		return err
	}

	if err := database.Callback().Row().Before("gorm:row").Register(
		"mpp:query_observer:row:start",
		startQueryObservation,
	); err != nil {
		return err
	}
	if err := database.Callback().Row().After("gorm:row").Register(
		"mpp:query_observer:row:finish",
		finishQueryObservation(p.observer, "row"),
	); err != nil {
		return err
	}

	if err := database.Callback().Raw().Before("gorm:raw").Register(
		"mpp:query_observer:raw:start",
		startQueryObservation,
	); err != nil {
		return err
	}
	return database.Callback().Raw().After("gorm:raw").Register(
		"mpp:query_observer:raw:finish",
		finishQueryObservation(p.observer, "raw"),
	)
}

func startQueryObservation(tx *gorm.DB) {
	tx.InstanceSet(queryObserverStartedAtKey, time.Now())
}

func finishQueryObservation(observer QueryObserver, operation string) func(*gorm.DB) {
	return func(tx *gorm.DB) {
		startedAt, ok := queryObservationStartedAt(tx)
		if !ok {
			return
		}

		sql := normalizedSQL(tx.Statement.SQL.String())
		if sql == "" {
			return
		}

		ctx := tx.Statement.Context
		if ctx == nil {
			ctx = context.Background()
		}

		duration := time.Since(startedAt)
		if duration <= 0 {
			duration = time.Nanosecond
		}

		observer.ObserveQuery(ctx, QueryObservation{
			Operation:    operation,
			Table:        normalizedTable(tx.Statement.Table),
			SQL:          sql,
			QueryHash:    hashSQL(sql),
			Duration:     duration,
			RowsAffected: tx.RowsAffected,
			Err:          tx.Error,
		})
	}
}

func queryObservationStartedAt(tx *gorm.DB) (time.Time, bool) {
	raw, ok := tx.InstanceGet(queryObserverStartedAtKey)
	if !ok {
		return time.Time{}, false
	}
	startedAt, ok := raw.(time.Time)
	return startedAt, ok
}

func normalizedTable(table string) string {
	table = strings.TrimSpace(table)
	if table == "" {
		return "unknown"
	}
	return table
}

func normalizedSQL(sql string) string {
	return strings.Join(strings.Fields(sql), " ")
}

func hashSQL(sql string) string {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(sql))
	return strconv.FormatUint(hash.Sum64(), 16)
}

package db

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type ReadConsistency string

const (
	StrongRead    ReadConsistency = "strong_read"
	ReadYourWrite ReadConsistency = "read_your_write"
	EventualRead  ReadConsistency = "eventual_read"
	AnalyticsRead ReadConsistency = "analytics_read"
)

type Router struct {
	writer *gorm.DB
	reader *gorm.DB
}

type RouterOption func(*Router)

func NewRouter(writer *gorm.DB, options ...RouterOption) *Router {
	router := &Router{
		writer: writer,
	}
	for _, option := range options {
		option(router)
	}
	return router
}

func WithReader(reader *gorm.DB) RouterOption {
	return func(router *Router) {
		if reader != nil {
			router.reader = reader
		}
	}
}

func (r *Router) Writer(ctx context.Context) *gorm.DB {
	if r == nil {
		return nil
	}
	return dbWithContext(r.writer, ctx)
}

func (r *Router) Reader(ctx context.Context, consistency ReadConsistency) *gorm.DB {
	if r == nil {
		return nil
	}
	switch consistency {
	case EventualRead, AnalyticsRead:
		if r.reader != nil && !stickyWriterActive(ctx, time.Now()) {
			return dbWithContext(r.reader, ctx)
		}
	}
	return r.Writer(ctx)
}

func (r *Router) HasReader() bool {
	return r != nil && r.reader != nil
}

func (r *Router) InstallQueryObserver(observer QueryObserver) error {
	if r == nil {
		return nil
	}
	if err := InstallQueryObserver(r.writer, observer); err != nil {
		return err
	}
	if r.reader == nil || r.reader == r.writer {
		return nil
	}
	return InstallQueryObserver(r.reader, observer)
}

func dbWithContext(database *gorm.DB, ctx context.Context) *gorm.DB {
	if database == nil || ctx == nil {
		return database
	}
	return database.WithContext(ctx)
}

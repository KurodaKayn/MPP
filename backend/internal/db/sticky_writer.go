package db

import (
	"context"
	"time"
)

type stickyWriterContextKey struct{}

// WithStickyWriter returns a context that forces eventual reads back to writer until the
// supplied deadline. Expired deadlines are ignored by the router.
func WithStickyWriter(ctx context.Context, until time.Time) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if until.IsZero() {
		return ctx
	}
	return context.WithValue(ctx, stickyWriterContextKey{}, until)
}

func StickyWriterUntil(ctx context.Context) (time.Time, bool) {
	if ctx == nil {
		return time.Time{}, false
	}
	until, ok := ctx.Value(stickyWriterContextKey{}).(time.Time)
	if !ok || until.IsZero() {
		return time.Time{}, false
	}
	return until, true
}

func stickyWriterActive(ctx context.Context, now time.Time) bool {
	until, ok := StickyWriterUntil(ctx)
	return ok && until.After(now)
}

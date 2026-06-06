package db

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

func TestRouterUsesReaderForEventualReads(t *testing.T) {
	writer := newRouterTestDB(t)
	reader := newRouterTestDB(t)

	require.NoError(t, writer.Create(&models.User{Username: "writer", Email: "writer@example.com", PasswordHash: "hash"}).Error)
	require.NoError(t, reader.Create(&models.User{Username: "reader-a", Email: "reader-a@example.com", PasswordHash: "hash"}).Error)
	require.NoError(t, reader.Create(&models.User{Username: "reader-b", Email: "reader-b@example.com", PasswordHash: "hash"}).Error)

	router := NewRouter(writer, WithReader(reader))

	var eventualCount int64
	require.NoError(t, router.Reader(context.Background(), EventualRead).Model(&models.User{}).Count(&eventualCount).Error)
	require.Equal(t, int64(2), eventualCount)

	var strongCount int64
	require.NoError(t, router.Reader(context.Background(), StrongRead).Model(&models.User{}).Count(&strongCount).Error)
	require.Equal(t, int64(1), strongCount)
}

func TestRouterFallsBackToWriterWithoutReader(t *testing.T) {
	writer := newRouterTestDB(t)

	require.NoError(t, writer.Create(&models.User{Username: "writer", Email: "writer@example.com", PasswordHash: "hash"}).Error)

	router := NewRouter(writer)

	var count int64
	require.NoError(t, router.Reader(context.Background(), EventualRead).Model(&models.User{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestRouterUsesWriterForStickyEventualReads(t *testing.T) {
	writer := newRouterTestDB(t)
	reader := newRouterTestDB(t)

	require.NoError(t, writer.Create(&models.User{Username: "writer", Email: "writer@example.com", PasswordHash: "hash"}).Error)
	require.NoError(t, reader.Create(&models.User{Username: "reader-a", Email: "reader-a@example.com", PasswordHash: "hash"}).Error)
	require.NoError(t, reader.Create(&models.User{Username: "reader-b", Email: "reader-b@example.com", PasswordHash: "hash"}).Error)

	router := NewRouter(writer, WithReader(reader))
	ctx := WithStickyWriter(context.Background(), time.Now().Add(time.Second))

	var count int64
	require.NoError(t, router.Reader(ctx, EventualRead).Model(&models.User{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestRouterUsesReaderAfterStickyWriterExpires(t *testing.T) {
	writer := newRouterTestDB(t)
	reader := newRouterTestDB(t)

	require.NoError(t, writer.Create(&models.User{Username: "writer", Email: "writer@example.com", PasswordHash: "hash"}).Error)
	require.NoError(t, reader.Create(&models.User{Username: "reader-a", Email: "reader-a@example.com", PasswordHash: "hash"}).Error)
	require.NoError(t, reader.Create(&models.User{Username: "reader-b", Email: "reader-b@example.com", PasswordHash: "hash"}).Error)

	router := NewRouter(writer, WithReader(reader))
	ctx := WithStickyWriter(context.Background(), time.Now().Add(-time.Second))

	var count int64
	require.NoError(t, router.Reader(ctx, EventualRead).Model(&models.User{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestStickyWriterUntilReturnsContextDeadline(t *testing.T) {
	until := time.Now().Add(time.Minute).UTC()
	ctx := WithStickyWriter(context.Background(), until)

	got, ok := StickyWriterUntil(ctx)
	require.True(t, ok)
	require.True(t, got.Equal(until))
}

func newRouterTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&models.User{}))
	return database
}

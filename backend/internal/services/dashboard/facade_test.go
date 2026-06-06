package dashboard_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestDashboardServiceWithContextIsolatesStickyWriterAcrossConcurrentScopes(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	limitDashboardFacadeDBConnections(t, writer)
	limitDashboardFacadeDBConnections(t, reader)
	router := dbrouter.NewRouter(writer, dbrouter.WithReader(reader))
	seedDashboardFacadeStats(t, writer, "writer", 1, models.PublicationStatusPublished)
	seedDashboardFacadeStats(t, reader, "reader", 2, models.PublicationStatusFailed)

	service := services.NewDashboardServiceWithRouter(writer, router)
	stickyCtx := dbrouter.WithStickyWriter(context.Background(), time.Now().Add(time.Minute))

	stickyService := service.WithContext(stickyCtx)
	readerService := service.WithContext(context.Background())
	requireStats(t, stickyService, expectedStickyWriterStats())
	requireStats(t, readerService, expectedReplicaStats())

	const pairs = 25
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, pairs*2)
	for range pairs {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			stats, err := service.WithContext(stickyCtx).GetStats(nil)
			if err != nil {
				errs <- err
				return
			}
			if err := compareStats(stats, expectedStickyWriterStats()); err != nil {
				errs <- err
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			stats, err := service.WithContext(context.Background()).GetStats(nil)
			if err != nil {
				errs <- err
				return
			}
			if err := compareStats(stats, expectedReplicaStats()); err != nil {
				errs <- err
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

func limitDashboardFacadeDBConnections(t *testing.T, db *gorm.DB) {
	t.Helper()

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
}

func seedDashboardFacadeStats(t *testing.T, db *gorm.DB, prefix string, projects int, publicationStatus string) {
	t.Helper()

	user := models.User{
		ID:           uuid.New(),
		Username:     prefix + "-user",
		Email:        prefix + "-user@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, db.Create(&user).Error)
	for range projects {
		project := models.Project{
			ID:            uuid.New(),
			UserID:        user.ID,
			Title:         prefix + "-project",
			SourceContent: "content",
			Status:        models.ProjectStatusReady,
		}
		require.NoError(t, db.Create(&project).Error)
		require.NoError(t, db.Create(&models.ProjectPlatformPublication{
			ID:        uuid.New(),
			ProjectID: project.ID,
			Platform:  "wechat",
			Status:    publicationStatus,
		}).Error)
	}
}

func requireStats(t *testing.T, service *services.DashboardService, expected dto.DashboardStatsResponse) {
	t.Helper()

	stats, err := service.GetStats(nil)
	require.NoError(t, err)
	require.NoError(t, compareStats(stats, expected))
}

func compareStats(stats *dto.DashboardStatsResponse, expected dto.DashboardStatsResponse) error {
	if stats.TotalUsers != expected.TotalUsers {
		return errDashboardStatsMismatch("total users", stats.TotalUsers, expected.TotalUsers)
	}
	if stats.TotalProjects != expected.TotalProjects {
		return errDashboardStatsMismatch("total projects", stats.TotalProjects, expected.TotalProjects)
	}
	if stats.TotalPublishedPublications != expected.TotalPublishedPublications {
		return errDashboardStatsMismatch("published publications", stats.TotalPublishedPublications, expected.TotalPublishedPublications)
	}
	if stats.TotalFailedPublications != expected.TotalFailedPublications {
		return errDashboardStatsMismatch("failed publications", stats.TotalFailedPublications, expected.TotalFailedPublications)
	}
	return nil
}

func errDashboardStatsMismatch(field string, got int64, expected int64) error {
	return fmt.Errorf("%s mismatch: got %d, expected %d", field, got, expected)
}

func expectedStickyWriterStats() dto.DashboardStatsResponse {
	return dto.DashboardStatsResponse{
		TotalUsers:                 1,
		TotalProjects:              1,
		TotalPublishedPublications: 1,
		TotalFailedPublications:    0,
	}
}

func expectedReplicaStats() dto.DashboardStatsResponse {
	return dto.DashboardStatsResponse{
		TotalUsers:                 1,
		TotalProjects:              2,
		TotalPublishedPublications: 0,
		TotalFailedPublications:    2,
	}
}

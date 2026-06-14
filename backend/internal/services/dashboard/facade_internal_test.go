package dashboard

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	readmodelsvc "github.com/kurodakayn/mpp-backend/internal/services/readmodel"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

type countingDashboardSideEffectTarget struct {
	projectListInvalidations int
	statsInvalidations       int
	projectRefreshes         int
	workspaceRefreshes       int
}

func (t *countingDashboardSideEffectTarget) InvalidateDashboardProjectListCache(context.Context) {
	t.projectListInvalidations++
}

func (t *countingDashboardSideEffectTarget) InvalidateDashboardStatsCache(context.Context) {
	t.statsInvalidations++
}

func (t *countingDashboardSideEffectTarget) RefreshProjectAsync(context.Context, uuid.UUID) {
	t.projectRefreshes++
}

func (t *countingDashboardSideEffectTarget) RefreshWorkspaceAsync(context.Context, uuid.UUID) {
	t.workspaceRefreshes++
}

type blockingDashboardReadModelRebuildQueue struct{}

func (blockingDashboardReadModelRebuildQueue) EnqueueDashboardRebuild(context.Context) (readmodelsvc.DashboardRebuildTaskInfo, error) {
	return readmodelsvc.DashboardRebuildTaskInfo{}, nil
}

func (blockingDashboardReadModelRebuildQueue) StartWorker(ctx context.Context, _ *readmodelsvc.Service) error {
	<-ctx.Done()
	return nil
}

func TestStartDashboardReadModelRebuildWorkerWithErrorsClosesOnContextCancel(t *testing.T) {
	service := NewDashboardService(testsupport.SetupTestDB())
	service.readModelRebuildQueue = blockingDashboardReadModelRebuildQueue{}
	ctx, cancel := context.WithCancel(context.Background())

	workerErrors := service.StartDashboardReadModelRebuildWorkerWithErrors(ctx)
	require.NotNil(t, workerErrors)
	cancel()

	select {
	case _, ok := <-workerErrors:
		require.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for dashboard read model worker errors channel to close")
	}
}

func TestDashboardSideEffectsFanOutThroughNarrowInterfaces(t *testing.T) {
	target := &countingDashboardSideEffectTarget{}
	effects := dashboardSideEffects{
		projectLists: target,
		stats:        target,
		readModels:   target,
	}

	effects.InvalidateDashboardProjectListCache(context.Background())
	effects.InvalidateDashboardStatsCache(context.Background())
	effects.RefreshProjectAsync(context.Background(), uuid.New())
	effects.RefreshWorkspaceAsync(context.Background(), uuid.New())

	require.Equal(t, 1, target.projectListInvalidations)
	require.Equal(t, 1, target.statsInvalidations)
	require.Equal(t, 1, target.projectRefreshes)
	require.Equal(t, 1, target.workspaceRefreshes)
}

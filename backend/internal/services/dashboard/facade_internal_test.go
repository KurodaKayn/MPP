package dashboard

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	readmodelsvc "github.com/kurodakayn/mpp-backend/internal/services/readmodel"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

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

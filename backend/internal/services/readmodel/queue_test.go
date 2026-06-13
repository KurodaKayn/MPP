package readmodel

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestDashboardRebuildTaskRoundTrip(t *testing.T) {
	task, err := newDashboardRebuildTask()
	require.NoError(t, err)
	require.Equal(t, dashboardRebuildTaskType, task.Type())
	require.NoError(t, dashboardRebuildJobFromTask(task))
}

func TestDashboardRebuildJobRejectsInvalidPayload(t *testing.T) {
	err := dashboardRebuildJobFromTask(asynq.NewTask(dashboardRebuildTaskType, []byte("{")))
	require.Error(t, err)
}

func TestRedisDashboardRebuildQueueEnqueuesUniqueTask(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	queue := NewRedisDashboardRebuildQueue(client)
	first, err := queue.EnqueueDashboardRebuild(context.Background())
	require.NoError(t, err)
	require.Equal(t, dashboardRebuildQueueName, first.Queue)
	require.Equal(t, dashboardRebuildTaskType, first.Type)
	require.False(t, first.Duplicate)

	duplicate, err := queue.EnqueueDashboardRebuild(context.Background())
	require.NoError(t, err)
	require.True(t, duplicate.Duplicate)
	require.Equal(t, "duplicate", duplicate.State)

	inspector := asynq.NewInspectorFromRedisClient(client)
	tasks, err := inspector.ListPendingTasks(dashboardRebuildQueueName)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, dashboardRebuildTaskType, tasks[0].Type)
	require.Equal(t, dashboardRebuildTaskMaxRetry, tasks[0].MaxRetry)
	require.Equal(t, dashboardRebuildTaskTimeout, tasks[0].Timeout)
}

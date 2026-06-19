package readmodel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

const (
	dashboardRebuildTaskType         = "readmodel:dashboard:rebuild"
	dashboardRebuildQueueName        = "readmodel"
	dashboardRebuildTaskMaxRetry     = 2
	dashboardRebuildTaskTimeout      = time.Hour
	dashboardRebuildTaskRetention    = 24 * time.Hour
	dashboardRebuildTaskUniqueTTL    = 30 * time.Minute
	dashboardRebuildEnqueueTimeout   = 5 * time.Second
	dashboardRebuildWorkerConcurrent = 1
)

var ErrDashboardRebuildQueueUnavailable = errors.New("dashboard read model rebuild queue unavailable")

type DashboardRebuildTaskInfo struct {
	TaskID    string `json:"task_id,omitempty"`
	Queue     string `json:"queue"`
	Type      string `json:"type"`
	State     string `json:"state"`
	Duplicate bool   `json:"duplicate"`
}

type RedisDashboardRebuildQueue struct {
	redisClient redis.UniversalClient
	asynqClient *asynq.Client
}

func NewRedisDashboardRebuildQueue(client redis.UniversalClient) *RedisDashboardRebuildQueue {
	if client == nil {
		return nil
	}
	return &RedisDashboardRebuildQueue{
		redisClient: client,
		asynqClient: asynq.NewClientFromRedisClient(client),
	}
}

func (q *RedisDashboardRebuildQueue) EnqueueDashboardRebuild(ctx context.Context) (DashboardRebuildTaskInfo, error) {
	info := DashboardRebuildTaskInfo{
		Queue: dashboardRebuildQueueName,
		Type:  dashboardRebuildTaskType,
	}
	if q == nil || q.asynqClient == nil {
		return info, ErrDashboardRebuildQueueUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	enqueueCtx, cancel := context.WithTimeout(ctx, dashboardRebuildEnqueueTimeout)
	defer cancel()

	task, err := newDashboardRebuildTask()
	if err != nil {
		return info, err
	}
	taskInfo, err := q.asynqClient.EnqueueContext(
		enqueueCtx,
		task,
		asynq.Queue(dashboardRebuildQueueName),
		asynq.MaxRetry(dashboardRebuildTaskMaxRetry),
		asynq.Timeout(dashboardRebuildTaskTimeout),
		asynq.Retention(dashboardRebuildTaskRetention),
		asynq.Unique(dashboardRebuildTaskUniqueTTL),
	)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		info.State = "duplicate"
		info.Duplicate = true
		return info, nil
	}
	if err != nil {
		return info, err
	}
	info.TaskID = taskInfo.ID
	info.State = taskInfo.State.String()
	return info, nil
}

func (q *RedisDashboardRebuildQueue) StartWorker(ctx context.Context, service *Service) error {
	if service == nil {
		return nil
	}
	if q == nil || q.redisClient == nil {
		return ErrDashboardRebuildQueueUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}

	server := asynq.NewServerFromRedisClient(q.redisClient, asynq.Config{
		Concurrency: dashboardRebuildWorkerConcurrent,
		Queues: map[string]int{
			dashboardRebuildQueueName: 1,
		},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(dashboardRebuildTaskType, func(taskCtx context.Context, task *asynq.Task) error {
		if err := dashboardRebuildJobFromTask(task); err != nil {
			return fmt.Errorf("invalid dashboard read model rebuild payload: %w: %w", err, asynq.SkipRetry)
		}
		result, err := service.WithContext(taskCtx).RebuildDashboard()
		if err != nil {
			return err
		}
		log.Printf(
			"dashboard read model rebuild complete: projects=%d workspaces=%d orphan_project_summaries=%d orphan_workspace_stats=%d",
			result.ProjectsRefreshed,
			result.WorkspacesRefreshed,
			result.OrphanProjectSummariesDeleted,
			result.OrphanWorkspaceStatsDeleted,
		)
		return nil
	})

	go func() {
		<-ctx.Done()
		server.Shutdown()
	}()

	if err := server.Run(mux); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return nil
}

func newDashboardRebuildTask() (*asynq.Task, error) {
	payload, err := json.Marshal(struct{}{})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(dashboardRebuildTaskType, payload), nil
}

func dashboardRebuildJobFromTask(task *asynq.Task) error {
	var payload struct{}
	return json.Unmarshal(task.Payload(), &payload)
}

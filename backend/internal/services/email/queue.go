package email

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

const (
	codeEmailTaskType         = "email:code"
	codeEmailQueueName        = "email"
	codeEmailTaskMaxRetry     = 5
	codeEmailEnqueueTimeout   = 5 * time.Second
	codeEmailTaskTimeout      = 30 * time.Second
	codeEmailTaskRetention    = 24 * time.Hour
	codeEmailWorkerConcurrent = 4

	codeEmailKindVerification  = "verification"
	codeEmailKindPasswordReset = "password_reset"
)

type codeEmailJob struct {
	Kind string `json:"kind"`
	To   string `json:"to"`
	Code string `json:"code"`
}

type AsyncEmailService struct {
	redisClient *redis.Client
	asynqClient *asynq.Client
}

func NewAsyncEmailService(client *redis.Client) *AsyncEmailService {
	return &AsyncEmailService{
		redisClient: client,
		asynqClient: asynq.NewClientFromRedisClient(client),
	}
}

func (s *AsyncEmailService) SendVerificationCode(ctx context.Context, to, code string) error {
	return s.enqueueCodeEmail(ctx, codeEmailJob{
		Kind: codeEmailKindVerification,
		To:   to,
		Code: code,
	})
}

func (s *AsyncEmailService) SendPasswordResetCode(ctx context.Context, to, code string) error {
	return s.enqueueCodeEmail(ctx, codeEmailJob{
		Kind: codeEmailKindPasswordReset,
		To:   to,
		Code: code,
	})
}

func (s *AsyncEmailService) StartWorker(ctx context.Context, sender EmailService) error {
	if sender == nil {
		return nil
	}

	server := asynq.NewServerFromRedisClient(s.redisClient, asynq.Config{
		Concurrency: codeEmailWorkerConcurrent,
		Queues: map[string]int{
			codeEmailQueueName: 1,
		},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(codeEmailTaskType, func(taskCtx context.Context, task *asynq.Task) error {
		job, err := codeEmailJobFromTask(task)
		if err != nil {
			return fmt.Errorf("invalid email task payload: %w: %w", err, asynq.SkipRetry)
		}
		return sendCodeEmail(taskCtx, sender, job)
	})

	go func() {
		<-ctx.Done()
		server.Shutdown()
	}()

	if err := server.Run(mux); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Printf("email worker stopped with error: %v", err)
		return err
	}
	return nil
}

func (s *AsyncEmailService) enqueueCodeEmail(ctx context.Context, job codeEmailJob) error {
	if ctx == nil {
		ctx = context.Background()
	}
	task, err := newCodeEmailTask(job)
	if err != nil {
		return err
	}
	enqueueCtx, cancel := context.WithTimeout(ctx, codeEmailEnqueueTimeout)
	defer cancel()
	_, err = s.asynqClient.EnqueueContext(
		enqueueCtx,
		task,
		asynq.Queue(codeEmailQueueName),
		asynq.MaxRetry(codeEmailTaskMaxRetry),
		asynq.Timeout(codeEmailTaskTimeout),
		asynq.Retention(codeEmailTaskRetention),
	)
	return err
}

func newCodeEmailTask(job codeEmailJob) (*asynq.Task, error) {
	job.Kind = strings.TrimSpace(job.Kind)
	job.To = strings.TrimSpace(job.To)
	job.Code = strings.TrimSpace(job.Code)
	if !validCodeEmailKind(job.Kind) {
		return nil, fmt.Errorf("unknown email job kind %q", job.Kind)
	}
	if job.To == "" || job.Code == "" {
		return nil, fmt.Errorf("email recipient and code are required")
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(codeEmailTaskType, payload), nil
}

func codeEmailJobFromTask(task *asynq.Task) (codeEmailJob, error) {
	var job codeEmailJob
	if err := json.Unmarshal(task.Payload(), &job); err != nil {
		return codeEmailJob{}, err
	}
	job.Kind = strings.TrimSpace(job.Kind)
	job.To = strings.TrimSpace(job.To)
	job.Code = strings.TrimSpace(job.Code)
	if !validCodeEmailKind(job.Kind) {
		return codeEmailJob{}, fmt.Errorf("unknown email job kind %q", job.Kind)
	}
	if job.To == "" || job.Code == "" {
		return codeEmailJob{}, fmt.Errorf("email recipient and code are required")
	}
	return job, nil
}

func sendCodeEmail(ctx context.Context, sender EmailService, job codeEmailJob) error {
	switch job.Kind {
	case codeEmailKindVerification:
		return sender.SendVerificationCode(ctx, job.To, job.Code)
	case codeEmailKindPasswordReset:
		return sender.SendPasswordResetCode(ctx, job.To, job.Code)
	default:
		return fmt.Errorf("unknown email job kind %q: %w", job.Kind, asynq.SkipRetry)
	}
}

func validCodeEmailKind(kind string) bool {
	return kind == codeEmailKindVerification || kind == codeEmailKindPasswordReset
}

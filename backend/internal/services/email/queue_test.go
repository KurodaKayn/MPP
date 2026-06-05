package email

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodeEmailTaskRoundTrip(t *testing.T) {
	task, err := newCodeEmailTask(codeEmailJob{
		Kind: codeEmailKindVerification,
		To:   " user@example.com ",
		Code: " 123456 ",
	})
	require.NoError(t, err)
	assert.Equal(t, codeEmailTaskType, task.Type())

	job, err := codeEmailJobFromTask(task)
	require.NoError(t, err)
	assert.Equal(t, codeEmailKindVerification, job.Kind)
	assert.Equal(t, "user@example.com", job.To)
	assert.Equal(t, "123456", job.Code)
}

func TestNewCodeEmailTaskRejectsMissingFields(t *testing.T) {
	_, err := newCodeEmailTask(codeEmailJob{
		Kind: codeEmailKindVerification,
		To:   "",
		Code: "123456",
	})
	require.Error(t, err)

	_, err = newCodeEmailTask(codeEmailJob{
		Kind: codeEmailKindVerification,
		To:   "user@example.com",
		Code: "",
	})
	require.Error(t, err)
}

func TestCodeEmailJobFromTaskRejectsInvalidPayload(t *testing.T) {
	_, err := codeEmailJobFromTask(asynq.NewTask(codeEmailTaskType, []byte("{")))
	require.Error(t, err)

	_, err = codeEmailJobFromTask(asynq.NewTask(codeEmailTaskType, []byte(`{"kind":"verification","to":"","code":"123456"}`)))
	require.Error(t, err)
}

func TestSendCodeEmailDispatchesByKind(t *testing.T) {
	mockEmail := &MockEmailService{}

	require.NoError(t, sendCodeEmail(context.Background(), mockEmail, codeEmailJob{
		Kind: codeEmailKindVerification,
		To:   "verify@example.com",
		Code: "123456",
	}))
	assert.Equal(t, "verify@example.com", mockEmail.LastTo)
	assert.Equal(t, "123456", mockEmail.LastBody)

	require.NoError(t, sendCodeEmail(context.Background(), mockEmail, codeEmailJob{
		Kind: codeEmailKindPasswordReset,
		To:   "reset@example.com",
		Code: "654321",
	}))
	assert.Equal(t, "reset@example.com", mockEmail.LastTo)
	assert.Equal(t, "654321", mockEmail.LastBody)
}

func TestSendCodeEmailSkipsRetryForUnknownKind(t *testing.T) {
	err := sendCodeEmail(context.Background(), &MockEmailService{}, codeEmailJob{
		Kind: "unknown",
		To:   "user@example.com",
		Code: "123456",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, asynq.SkipRetry)
}

func TestAsyncEmailServiceEnqueuesCodeEmail(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	service := NewAsyncEmailService(client)
	require.NoError(t, service.SendVerificationCode(context.Background(), "user@example.com", "123456"))

	inspector := asynq.NewInspectorFromRedisClient(client)
	tasks, err := inspector.ListPendingTasks(codeEmailQueueName)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, codeEmailTaskType, tasks[0].Type)

	job, err := codeEmailJobFromTask(asynq.NewTask(tasks[0].Type, tasks[0].Payload))
	require.NoError(t, err)
	assert.Equal(t, codeEmailKindVerification, job.Kind)
	assert.Equal(t, "user@example.com", job.To)
	assert.Equal(t, "123456", job.Code)
}

func TestNewCodeEmailTaskRejectsUnknownKind(t *testing.T) {
	_, err := newCodeEmailTask(codeEmailJob{
		Kind: "unknown",
		To:   "user@example.com",
		Code: "123456",
	})
	require.Error(t, err)
}

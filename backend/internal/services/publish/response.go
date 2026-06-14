package publish

import (
	"time"

	"github.com/google/uuid"
)

type PublishResponse struct {
	Status                 string     `json:"status"`
	Message                string     `json:"message,omitempty"`
	Platform               string     `json:"platform,omitempty"`
	RemoteID               string     `json:"remote_id,omitempty"`
	PublishURL             string     `json:"publish_url,omitempty"`
	ErrorMessage           string     `json:"error_message,omitempty"`
	BrowserSessionID       uuid.UUID  `json:"browser_session_id,omitempty"`
	JobID                  string     `json:"job_id,omitempty"`
	IdempotencyKey         string     `json:"idempotency_key,omitempty"`
	QueuedAt               *time.Time `json:"queued_at,omitempty"`
	ScheduledPublicationID string     `json:"scheduled_publication_id,omitempty"`
	ScheduledAt            *time.Time `json:"scheduled_at,omitempty"`
}

func publishErrorResponse(err error) PublishResponse {
	return PublishResponse{Status: "error", Message: err.Error()}
}

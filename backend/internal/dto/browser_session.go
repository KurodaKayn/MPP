package dto

import (
	"time"

	"github.com/google/uuid"
)

type StartBrowserSessionResponse struct {
	SessionID            uuid.UUID `json:"session_id"`
	Status               string    `json:"status"`
	StreamURL            string    `json:"stream_url"`
	StreamTokenExpiresAt time.Time `json:"stream_token_expires_at"`
	ExpiresAt            time.Time `json:"expires_at"`
}

type BrowserSessionResponse struct {
	SessionID            uuid.UUID `json:"session_id"`
	Platform             string    `json:"platform"`
	Status               string    `json:"status"`
	StreamURL            string    `json:"stream_url,omitempty"`
	StreamTokenExpiresAt time.Time `json:"stream_token_expires_at,omitempty"`
	ExpiresAt            time.Time `json:"expires_at"`
	Message              string    `json:"message"`
}

type CompleteBrowserSessionResponse struct {
	SessionID uuid.UUID `json:"session_id"`
	Platform  string    `json:"platform"`
	Status    string    `json:"status"`
	Account   struct {
		Username  string `json:"username"`
		AvatarURL string `json:"avatar_url"`
	} `json:"account"`
	Message string `json:"message"`
}

type CancelBrowserSessionResponse struct {
	SessionID uuid.UUID `json:"session_id"`
	Status    string    `json:"status"`
}

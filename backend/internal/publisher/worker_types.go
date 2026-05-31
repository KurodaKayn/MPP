package publisher

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type DomainRule struct {
	Host    string   `json:"host"`
	Match   string   `json:"match"` // "exact" or "suffix"
	Schemes []string `json:"schemes"`
	Purpose string   `json:"purpose"`
}

type CookieRequirement struct {
	Name           string   `json:"name"`
	DomainSuffixes []string `json:"domain_suffixes"`
	Required       bool     `json:"required"`
	Preserve       bool     `json:"preserve"`
}

type StartWorkerSessionRequest struct {
	SessionID       uuid.UUID           `json:"session_id"`
	UserID          uuid.UUID           `json:"user_id"`
	Platform        string              `json:"platform"`
	LoginURL        string              `json:"login_url"`
	AllowedDomains  []DomainRule        `json:"allowed_domains"`
	RequiredCookies []CookieRequirement `json:"required_cookies"`
	TTLSeconds      int                 `json:"ttl_seconds"`
	Viewport        struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"viewport"`
}

type StartWorkerSessionResponse struct {
	WorkerSessionRef  string    `json:"worker_session_ref"`
	Status            string    `json:"status"`
	ContainerID       string    `json:"container_id"`
	CDPEndpointRef    string    `json:"cdp_endpoint_ref"`
	StreamEndpointRef string    `json:"stream_endpoint_ref"`
	StartedAt         time.Time `json:"started_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

type GetWorkerSessionResponse struct {
	WorkerSessionRef string   `json:"worker_session_ref"`
	Status           string   `json:"status"`
	CurrentURL       string   `json:"current_url"`
	LoginDetected    bool     `json:"login_detected"`
	MissingCookies   []string `json:"missing_cookies"`
	Message          string   `json:"message"`
}

type CaptureWorkerSessionResponse struct {
	Status         string               `json:"status"`
	Cookies        []Cookie             `json:"cookies"`
	MissingCookies []string             `json:"missing_cookies,omitempty"`
	Account        RemoteAccountProfile `json:"account"`
}

type BrowserWorkerClient interface {
	CreateSession(ctx context.Context, req StartWorkerSessionRequest) (*StartWorkerSessionResponse, error)
	GetSession(ctx context.Context, ref string) (*GetWorkerSessionResponse, error)
	CaptureSession(ctx context.Context, ref string) (*CaptureWorkerSessionResponse, error)
	StopSession(ctx context.Context, ref string) error
}

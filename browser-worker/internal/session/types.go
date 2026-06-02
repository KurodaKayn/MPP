package session

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	Secure   bool    `json:"secure"`
	HttpOnly bool    `json:"httpOnly"`
	SameSite string  `json:"sameSite"`
}

type RemoteAccountProfile struct {
	PlatformUserID string `json:"platform_user_id"`
	Username       string `json:"username"`
	AvatarURL      string `json:"avatar_url"`
}

type CaptureWorkerSessionResponse struct {
	Status         string               `json:"status"`
	Cookies        []Cookie             `json:"cookies"`
	MissingCookies []string             `json:"missing_cookies,omitempty"`
	Account        RemoteAccountProfile `json:"account"`
}

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
	InitialCookies  []Cookie            `json:"initial_cookies,omitempty"`
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

type WorkerSession struct {
	ID                string
	SessionID         uuid.UUID
	UserID            uuid.UUID
	Platform          string
	Status            string
	ContainerID       string
	CDPEndpointRef    string
	StreamEndpointRef string
	InternalStreamURL string
	RequiredCookies   []CookieRequirement
	CreatedAt         time.Time
	ExpiresAt         time.Time
	BrowserContext    context.Context
	CDPMu             sync.Mutex
	StateCancel       context.CancelFunc
	StateStore        *RedisStateStore
	CancelFunc        context.CancelFunc
}

package session

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-browser-worker/internal/contracts"
	browserruntime "github.com/kurodakayn/mpp-browser-worker/internal/runtime"
)

type Cookie = contracts.BrowserWorkerCookie
type CookieRequirement = contracts.BrowserWorkerCookieRequirement
type DomainRule = contracts.BrowserWorkerDomainRule
type RemoteAccountProfile = contracts.BrowserWorkerRemoteAccountProfile
type CaptureWorkerSessionResponse = contracts.BrowserWorkerCaptureSessionResponse
type StartWorkerSessionRequest = contracts.BrowserWorkerStartSessionRequest
type StartWorkerSessionResponse = contracts.BrowserWorkerStartSessionResponse

type WorkerSession struct {
	ID                string
	SessionID         uuid.UUID
	UserID            uuid.UUID
	Platform          string
	Status            string
	RuntimeReference  browserruntime.SessionReference
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

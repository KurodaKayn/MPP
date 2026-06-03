package browser

import (
	"context"

	"github.com/kurodakayn/mpp-backend/internal/contracts"
)

type Cookie = contracts.BrowserWorkerCookie
type CookieRequirement = contracts.BrowserWorkerCookieRequirement
type DomainRule = contracts.BrowserWorkerDomainRule
type RemoteAccountProfile = contracts.BrowserWorkerRemoteAccountProfile
type StartWorkerSessionRequest = contracts.BrowserWorkerStartSessionRequest
type StartWorkerSessionResponse = contracts.BrowserWorkerStartSessionResponse
type GetWorkerSessionResponse = contracts.BrowserWorkerGetSessionResponse
type CaptureWorkerSessionResponse = contracts.BrowserWorkerCaptureSessionResponse
type StartDouyinPublishRequest = contracts.BrowserWorkerStartDouyinPublishRequest

type BrowserWorkerClient interface {
	CreateSession(ctx context.Context, req StartWorkerSessionRequest) (*StartWorkerSessionResponse, error)
	GetSession(ctx context.Context, ref string) (*GetWorkerSessionResponse, error)
	CaptureSession(ctx context.Context, ref string) (*CaptureWorkerSessionResponse, error)
	StartDouyinPublish(ctx context.Context, ref string, req StartDouyinPublishRequest) error
	StopSession(ctx context.Context, ref string) error
}

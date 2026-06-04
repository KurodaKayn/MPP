package services

import (
	"github.com/kurodakayn/mpp-backend/internal/services/ai"
	collabdoc "github.com/kurodakayn/mpp-backend/internal/services/collabdoc"
	dashboard "github.com/kurodakayn/mpp-backend/internal/services/dashboard"
	platformaccount "github.com/kurodakayn/mpp-backend/internal/services/platform_account"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
)

type AIContentEditor = ai.AIContentEditor
type AIServiceClient = ai.AIServiceClient
type AIServiceStream = ai.AIServiceStream

type CollabDocumentService = collabdoc.Service
type DashboardService = dashboard.DashboardService
type MemoryXOAuth2StateStore = platformaccount.MemoryXOAuth2StateStore
type PublishJob = publishsvc.PublishJob
type PublishQueue = publishsvc.PublishQueue
type RedisPublishQueue = publishsvc.RedisPublishQueue
type RedisXOAuth2StateStore = platformaccount.RedisXOAuth2StateStore
type WechatAPITester = platformaccount.WechatAPITester
type WechatConnectionTester = platformaccount.WechatConnectionTester
type XAPITester = platformaccount.XAPITester
type XConnectionTester = platformaccount.XConnectionTester
type XOAuth2API = platformaccount.XOAuth2API
type XOAuth2Provider = platformaccount.XOAuth2Provider
type XOAuth2StateStore = platformaccount.XOAuth2StateStore

var ErrAIServiceUnavailable = ai.ErrAIServiceUnavailable
var ErrExtensionCallbackTokenExpired = dashboard.ErrExtensionCallbackTokenExpired
var ErrExtensionCallbackTokenInvalid = dashboard.ErrExtensionCallbackTokenInvalid
var ErrForbidden = dashboard.ErrForbidden
var ErrCollabDocumentForbidden = collabdoc.ErrDocumentForbidden
var ErrInvalidCollabDocument = collabdoc.ErrInvalidDocument
var ErrInvalidAIEditRequest = ai.ErrInvalidAIEditRequest
var ErrInvalidPlatformAccount = platformaccount.ErrInvalidPlatformAccount
var ErrInvalidProject = dashboard.ErrInvalidProject
var ErrInvalidXOAuth2State = platformaccount.ErrInvalidXOAuth2State
var ErrManualPublishUnsupported = dashboard.ErrManualPublishUnsupported
var ErrPublicationAlreadyPublishing = publishsvc.ErrPublicationAlreadyPublishing
var ErrPublicationDisabled = dashboard.ErrPublicationDisabled
var ErrPublicationRequiresSync = dashboard.ErrPublicationRequiresSync
var ErrPublishQueueEmpty = publishsvc.ErrPublishQueueEmpty
var ErrXOAuth2NotConfigured = platformaccount.ErrXOAuth2NotConfigured

var NewAIServiceClient = ai.NewAIServiceClient
var NewAIServiceClientFromEnv = ai.NewAIServiceClientFromEnv
var NewCollabDocumentService = collabdoc.NewService
var NewDashboardService = dashboard.NewDashboardService
var NewDashboardServiceWithPlatformTesters = dashboard.NewDashboardServiceWithPlatformTesters
var NewDashboardServiceWithWechatTester = dashboard.NewDashboardServiceWithWechatTester
var NewDashboardServiceWithXOAuth2Provider = dashboard.NewDashboardServiceWithXOAuth2Provider
var NewMemoryXOAuth2StateStore = platformaccount.NewMemoryXOAuth2StateStore
var NewRedisPublishQueue = publishsvc.NewRedisPublishQueue
var NewRedisXOAuth2StateStore = platformaccount.NewRedisXOAuth2StateStore

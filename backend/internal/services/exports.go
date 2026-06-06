package services

import (
	"github.com/kurodakayn/mpp-backend/internal/services/ai"
	collabdoc "github.com/kurodakayn/mpp-backend/internal/services/collabdoc"
	dashboard "github.com/kurodakayn/mpp-backend/internal/services/dashboard"
	mediaassetsvc "github.com/kurodakayn/mpp-backend/internal/services/mediaasset"
	platformaccount "github.com/kurodakayn/mpp-backend/internal/services/platform_account"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
)

type AIContentEditor = ai.AIContentEditor
type AIServiceClient = ai.AIServiceClient
type AIServiceStream = ai.AIServiceStream

type CollabDocumentService = collabdoc.Service
type CollabDocumentSessionConfig = collabdoc.SessionConfig
type CollabDocumentSession = collabdoc.Session
type ProjectDocumentInitializer = collabdoc.ProjectDocumentInitializer
type DashboardService = dashboard.DashboardService
type MemoryXOAuth2StateStore = platformaccount.MemoryXOAuth2StateStore
type PublishJob = publishsvc.PublishJob
type PublishQueue = publishsvc.PublishQueue
type PublishRequest = publishsvc.PublishRequest
type WorkspacePermission = dashboard.WorkspacePermission
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
var ErrInvalidProjectComment = dashboard.ErrInvalidProjectComment
var ErrInvalidProjectCollaborator = dashboard.ErrInvalidProjectCollaborator
var ErrInvalidProjectShareLink = dashboard.ErrInvalidProjectShareLink
var ErrInvalidProjectVersion = dashboard.ErrInvalidProjectVersion
var ErrInvalidWorkspace = dashboard.ErrInvalidWorkspace
var ErrInvalidWorkspaceInvite = dashboard.ErrInvalidWorkspaceInvite
var ErrInvalidWorkspaceMember = dashboard.ErrInvalidWorkspaceMember
var ErrProjectCollabUnavailable = dashboard.ErrProjectCollabUnavailable
var ErrInvalidXOAuth2State = platformaccount.ErrInvalidXOAuth2State
var ErrManualPublishUnsupported = dashboard.ErrManualPublishUnsupported
var ErrInvalidMediaAsset = mediaassetsvc.ErrInvalidMediaAsset
var ErrMediaAssetNotReady = mediaassetsvc.ErrMediaAssetNotReady
var ErrMediaAssetUploadIncomplete = mediaassetsvc.ErrMediaAssetUploadIncomplete
var ErrMediaStorageUnavailable = mediaassetsvc.ErrMediaStorageUnavailable
var ErrPublicationAlreadyPublishing = publishsvc.ErrPublicationAlreadyPublishing
var ErrPublicationDisabled = dashboard.ErrPublicationDisabled
var ErrPublicationRequiresSync = dashboard.ErrPublicationRequiresSync
var ErrPublishMediaAssetNotReady = dashboard.ErrPublishMediaAssetNotReady
var ErrPublishQueueEmpty = publishsvc.ErrPublishQueueEmpty
var ErrXOAuth2NotConfigured = platformaccount.ErrXOAuth2NotConfigured

const PermissionAccountManage = dashboard.PermissionAccountManage

var NewAIServiceClient = ai.NewAIServiceClient
var NewAIServiceClientFromEnv = ai.NewAIServiceClientFromEnv
var NewCollabDocumentService = collabdoc.NewService
var NewHTTPProjectDocumentInitializer = collabdoc.NewHTTPProjectDocumentInitializer
var NewDashboardService = dashboard.NewDashboardService
var NewDashboardServiceWithRouter = dashboard.NewDashboardServiceWithRouter
var NewDashboardServiceWithPlatformTesters = dashboard.NewDashboardServiceWithPlatformTesters
var NewDashboardServiceWithWechatTester = dashboard.NewDashboardServiceWithWechatTester
var NewDashboardServiceWithXOAuth2Provider = dashboard.NewDashboardServiceWithXOAuth2Provider
var NewMemoryXOAuth2StateStore = platformaccount.NewMemoryXOAuth2StateStore
var NewRedisPublishQueue = publishsvc.NewRedisPublishQueue
var NewRedisXOAuth2StateStore = platformaccount.NewRedisXOAuth2StateStore

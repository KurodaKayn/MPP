package services

import (
	"github.com/kurodakayn/mpp-backend/internal/services/ai"
	collabdoc "github.com/kurodakayn/mpp-backend/internal/services/collabdoc"
	dashboard "github.com/kurodakayn/mpp-backend/internal/services/dashboard"
	extensionsvc "github.com/kurodakayn/mpp-backend/internal/services/extension"
	mediaassetsvc "github.com/kurodakayn/mpp-backend/internal/services/mediaasset"
	platformaccount "github.com/kurodakayn/mpp-backend/internal/services/platform_account"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
	readmodelsvc "github.com/kurodakayn/mpp-backend/internal/services/readmodel"
	workspacesvc "github.com/kurodakayn/mpp-backend/internal/services/workspace"
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
type DashboardRebuildTaskInfo = readmodelsvc.DashboardRebuildTaskInfo
type WorkspacePermission = workspacesvc.Permission
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
var ErrExtensionCallbackTokenExpired = extensionsvc.ErrExtensionCallbackTokenExpired
var ErrExtensionCallbackTokenInvalid = extensionsvc.ErrExtensionCallbackTokenInvalid
var ErrForbidden = publishsvc.ErrForbidden
var ErrCollabDocumentForbidden = collabdoc.ErrDocumentForbidden
var ErrInvalidCollabDocument = collabdoc.ErrInvalidDocument
var ErrInvalidAIEditRequest = ai.ErrInvalidAIEditRequest
var ErrInvalidPlatformAccount = platformaccount.ErrInvalidPlatformAccount
var ErrInvalidProject = projectsvc.ErrInvalidProject
var ErrInvalidProjectComment = projectsvc.ErrInvalidProjectComment
var ErrInvalidProjectCollaborator = projectsvc.ErrInvalidProjectCollaborator
var ErrInvalidProjectShareLink = projectsvc.ErrInvalidProjectShareLink
var ErrInvalidProjectVersion = projectsvc.ErrInvalidProjectVersion
var ErrInvalidWorkspace = workspacesvc.ErrInvalidWorkspace
var ErrInvalidWorkspaceInvite = workspacesvc.ErrInvalidWorkspaceInvite
var ErrInvalidWorkspaceMember = workspacesvc.ErrInvalidWorkspaceMember
var ErrProjectDeletionBlocked = projectsvc.ErrProjectDeletionBlocked
var ErrProjectCollabUnavailable = projectsvc.ErrProjectCollabUnavailable
var ErrInvalidXOAuth2State = platformaccount.ErrInvalidXOAuth2State
var ErrManualPublishUnsupported = publishsvc.ErrManualPublishUnsupported
var ErrInvalidMediaAsset = mediaassetsvc.ErrInvalidMediaAsset
var ErrMediaAssetNotReady = mediaassetsvc.ErrMediaAssetNotReady
var ErrMediaAssetUploadIncomplete = mediaassetsvc.ErrMediaAssetUploadIncomplete
var ErrMediaStorageUnavailable = mediaassetsvc.ErrMediaStorageUnavailable
var ErrPublicationAlreadyPublishing = publishsvc.ErrPublicationAlreadyPublishing
var ErrPublicationDisabled = publishsvc.ErrPublicationDisabled
var ErrPublicationRequiresSync = publishsvc.ErrPublicationRequiresSync
var ErrPublishMediaAssetNotReady = publishsvc.ErrPublishMediaAssetNotReady
var ErrPublishQueueEmpty = publishsvc.ErrPublishQueueEmpty
var ErrDashboardRebuildQueueUnavailable = readmodelsvc.ErrDashboardRebuildQueueUnavailable
var ErrXOAuth2NotConfigured = platformaccount.ErrXOAuth2NotConfigured

const PermissionAccountManage = workspacesvc.PermissionAccountManage

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

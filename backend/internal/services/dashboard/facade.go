package dashboard

import (
	"context"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	collabdoc "github.com/kurodakayn/mpp-backend/internal/services/collabdoc"
	"github.com/kurodakayn/mpp-backend/internal/services/compiler"
	extensionsvc "github.com/kurodakayn/mpp-backend/internal/services/extension"
	mediaassetsvc "github.com/kurodakayn/mpp-backend/internal/services/mediaasset"
	platformaccount "github.com/kurodakayn/mpp-backend/internal/services/platform_account"
	prepublishsvc "github.com/kurodakayn/mpp-backend/internal/services/prepublish"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
	statssvc "github.com/kurodakayn/mpp-backend/internal/services/stats"
	workspacesvc "github.com/kurodakayn/mpp-backend/internal/services/workspace"
)

var ErrForbidden = publishsvc.ErrForbidden
var ErrInvalidProject = projectsvc.ErrInvalidProject
var ErrInvalidProjectCollaborator = projectsvc.ErrInvalidProjectCollaborator
var ErrInvalidProjectComment = projectsvc.ErrInvalidProjectComment
var ErrInvalidProjectShareLink = projectsvc.ErrInvalidProjectShareLink
var ErrInvalidProjectVersion = projectsvc.ErrInvalidProjectVersion
var ErrInvalidWorkspace = workspacesvc.ErrInvalidWorkspace
var ErrInvalidWorkspaceInvite = workspacesvc.ErrInvalidWorkspaceInvite
var ErrInvalidWorkspaceMember = workspacesvc.ErrInvalidWorkspaceMember
var ErrProjectCollabUnavailable = projectsvc.ErrProjectCollabUnavailable
var ErrPublicationDisabled = publishsvc.ErrPublicationDisabled
var ErrPublicationRequiresSync = publishsvc.ErrPublicationRequiresSync
var ErrManualPublishUnsupported = publishsvc.ErrManualPublishUnsupported
var ErrExtensionCallbackTokenInvalid = extensionsvc.ErrExtensionCallbackTokenInvalid
var ErrExtensionCallbackTokenExpired = extensionsvc.ErrExtensionCallbackTokenExpired
var ErrMediaStorageUnavailable = mediaassetsvc.ErrMediaStorageUnavailable
var ErrInvalidMediaAsset = mediaassetsvc.ErrInvalidMediaAsset
var ErrMediaAssetUploadIncomplete = mediaassetsvc.ErrMediaAssetUploadIncomplete
var ErrMediaAssetNotReady = mediaassetsvc.ErrMediaAssetNotReady

type ProjectDraftCompiler = compiler.ProjectDraftCompiler
type WorkspacePermission = workspacesvc.Permission

const PermissionAccountManage = workspacesvc.PermissionAccountManage

type DashboardService struct {
	*Project
	*Workspace
	*Prepublish
	*Extension
	*MediaAsset
	*Stats
	*AccountSettings
	*Publisher

	db       *gorm.DB
	dbRouter *dbrouter.Router
}

func NewDashboardService(db *gorm.DB) *DashboardService {
	return NewDashboardServiceWithRouter(db, nil)
}

func NewDashboardServiceWithRouter(db *gorm.DB, router *dbrouter.Router) *DashboardService {
	return newDashboardServiceWithPlatformTesters(db, platformaccount.WechatAPITester{}, platformaccount.XAPITester{}, router)
}

func (s *DashboardService) WithContext(ctx context.Context) *DashboardService {
	if ctx == nil {
		return s
	}
	scoped := *s
	scoped.db = s.db.WithContext(ctx)
	scoped.Project.Service = s.Project.WithContext(ctx)
	scoped.Workspace.Service = workspacesvc.NewService(scoped.db, scoped.Project.Service)
	scoped.Prepublish.Service = prepublishsvc.NewService(scoped.db, scoped.Project.Service, s.ServiceDraftCompiler())
	scoped.Extension.Service = extensionsvc.NewService(scoped.db)
	scoped.MediaAsset.Service = s.MediaAsset.WithContext(ctx)
	scoped.Stats.Service = statssvc.NewServiceWithRouter(scoped.db, scoped.Project.Service, scoped.dbRouter)
	scoped.AccountSettings.Service = s.AccountSettings.WithContext(ctx)
	scoped.Publisher.Service = s.Publisher.WithContext(ctx)
	return &scoped
}

type Project struct {
	*projectsvc.Service
}

type Workspace struct {
	*workspacesvc.Service
}

type Prepublish struct {
	*prepublishsvc.Service
}

type Extension struct {
	*extensionsvc.Service
}

type MediaAsset struct {
	*mediaassetsvc.Service
}

type Stats struct {
	*statssvc.Service
}

type AccountSettings struct {
	*platformaccount.Service
}

type Publisher struct {
	*publishsvc.Service
}

func (p *Prepublish) ServiceDraftCompiler() ProjectDraftCompiler {
	if p == nil || p.Service == nil {
		return nil
	}
	return p.DraftCompiler()
}

func (s *DashboardService) SetBrowserWorkerClient(client publisher.BrowserWorkerClient) {
	s.Publisher.SetBrowserWorkerClient(client)
}

func (s *DashboardService) SetBrowserSessionService(svc *browsersession.BrowserSessionService) {
	s.Publisher.SetBrowserSessionService(svc)
}

func (s *DashboardService) SetPublishJobObserver(observer publishsvc.PublishJobObserver) {
	s.Publisher.SetPublishJobObserver(observer)
}

func (s *DashboardService) SetCollabDocumentService(svc *collabdoc.Service) {
	s.Project.SetCollabDocumentService(svc)
}

func (s *DashboardService) SetDraftCompiler(compiler ProjectDraftCompiler) {
	s.Prepublish.SetDraftCompiler(compiler)
}

func (s *DashboardService) UseObjectStorage(client objectstorage.Client, config objectstorage.Config) {
	s.MediaAsset.UseObjectStorage(client, config)
	s.Publisher.UseObjectStorage(client, config)
}

func NewDashboardServiceWithWechatTester(db *gorm.DB, tester platformaccount.WechatConnectionTester) *DashboardService {
	return NewDashboardServiceWithPlatformTesters(db, tester, platformaccount.XAPITester{})
}

func NewDashboardServiceWithPlatformTesters(db *gorm.DB, tester platformaccount.WechatConnectionTester, xTester platformaccount.XConnectionTester) *DashboardService {
	return newDashboardServiceWithPlatformTesters(db, tester, xTester, nil)
}

func newDashboardServiceWithPlatformTesters(db *gorm.DB, tester platformaccount.WechatConnectionTester, xTester platformaccount.XConnectionTester, router *dbrouter.Router) *DashboardService {
	if router == nil {
		router = dbrouter.NewRouter(db)
	}
	accounts := platformaccount.NewServiceWithPlatformTesters(db, tester, xTester)
	projects := projectsvc.NewService(db)
	publisher := publishsvc.NewService(db, accounts)
	return &DashboardService{
		Project:         &Project{Service: projects},
		Workspace:       &Workspace{Service: workspacesvc.NewService(db, projects)},
		Prepublish:      &Prepublish{Service: prepublishsvc.NewService(db, projects, compiler.NewContentPipelineDraftCompiler())},
		Extension:       &Extension{Service: extensionsvc.NewService(db)},
		MediaAsset:      &MediaAsset{Service: mediaassetsvc.NewService(db, projects)},
		Stats:           &Stats{Service: statssvc.NewServiceWithRouter(db, projects, router)},
		AccountSettings: &AccountSettings{Service: accounts},
		Publisher:       &Publisher{Service: publisher},
		db:              db,
		dbRouter:        router,
	}
}

func NewDashboardServiceWithXOAuth2Provider(db *gorm.DB, provider platformaccount.XOAuth2Provider) *DashboardService {
	accounts := platformaccount.NewServiceWithXOAuth2Provider(db, provider)
	projects := projectsvc.NewService(db)
	publisher := publishsvc.NewService(db, accounts)
	router := dbrouter.NewRouter(db)
	return &DashboardService{
		Project:         &Project{Service: projects},
		Workspace:       &Workspace{Service: workspacesvc.NewService(db, projects)},
		Prepublish:      &Prepublish{Service: prepublishsvc.NewService(db, projects, compiler.NewContentPipelineDraftCompiler())},
		Extension:       &Extension{Service: extensionsvc.NewService(db)},
		MediaAsset:      &MediaAsset{Service: mediaassetsvc.NewService(db, projects)},
		Stats:           &Stats{Service: statssvc.NewServiceWithRouter(db, projects, router)},
		AccountSettings: &AccountSettings{Service: accounts},
		Publisher:       &Publisher{Service: publisher},
		db:              db,
		dbRouter:        router,
	}
}

func (s *DashboardService) SetPublishQueue(queue publishsvc.PublishQueue) {
	s.SetQueue(queue)
}

func (s *DashboardService) UseRedis(client *redis.Client) {
	if client == nil {
		return
	}
	s.AccountSettings.UseRedis(client)
	s.Publisher.UseRedis(client)
}

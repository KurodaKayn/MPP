package dashboard

import (
	"context"

	"github.com/kurodakayn/mpp-backend/internal/publisher"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	collabdoc "github.com/kurodakayn/mpp-backend/internal/services/collabdoc"
	"github.com/kurodakayn/mpp-backend/internal/services/compiler"
	extensionsvc "github.com/kurodakayn/mpp-backend/internal/services/extension"
	platformaccount "github.com/kurodakayn/mpp-backend/internal/services/platform_account"
	prepublishsvc "github.com/kurodakayn/mpp-backend/internal/services/prepublish"
	projectsvc "github.com/kurodakayn/mpp-backend/internal/services/project"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
	statssvc "github.com/kurodakayn/mpp-backend/internal/services/stats"
	workspacesvc "github.com/kurodakayn/mpp-backend/internal/services/workspace"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var ErrForbidden = publishsvc.ErrForbidden
var ErrInvalidProject = projectsvc.ErrInvalidProject
var ErrInvalidProjectCollaborator = projectsvc.ErrInvalidProjectCollaborator
var ErrInvalidWorkspace = workspacesvc.ErrInvalidWorkspace
var ErrInvalidWorkspaceMember = workspacesvc.ErrInvalidWorkspaceMember
var ErrProjectCollabUnavailable = projectsvc.ErrProjectCollabUnavailable
var ErrPublicationDisabled = publishsvc.ErrPublicationDisabled
var ErrPublicationRequiresSync = publishsvc.ErrPublicationRequiresSync
var ErrManualPublishUnsupported = publishsvc.ErrManualPublishUnsupported
var ErrExtensionCallbackTokenInvalid = extensionsvc.ErrExtensionCallbackTokenInvalid
var ErrExtensionCallbackTokenExpired = extensionsvc.ErrExtensionCallbackTokenExpired

type ProjectDraftCompiler = compiler.ProjectDraftCompiler

type DashboardService struct {
	*Project
	*Workspace
	*Prepublish
	*Extension
	*Stats
	*AccountSettings
	*Publisher

	db *gorm.DB
}

func NewDashboardService(db *gorm.DB) *DashboardService {
	return NewDashboardServiceWithPlatformTesters(db, platformaccount.WechatAPITester{}, platformaccount.XAPITester{})
}

func (s *DashboardService) WithContext(ctx context.Context) *DashboardService {
	if ctx == nil {
		return s
	}
	scoped := *s
	scoped.db = s.db.WithContext(ctx)
	scoped.Project.Service = s.Project.Service.WithContext(ctx)
	scoped.Workspace.Service = workspacesvc.NewService(scoped.db, scoped.Project.Service)
	scoped.Prepublish.Service = prepublishsvc.NewService(scoped.db, scoped.Project.Service, s.Prepublish.ServiceDraftCompiler())
	scoped.Extension.Service = extensionsvc.NewService(scoped.db)
	scoped.Stats.Service = statssvc.NewService(scoped.db, scoped.Project.Service)
	scoped.AccountSettings.Service = s.AccountSettings.Service.WithContext(ctx)
	scoped.Publisher.Service = s.Publisher.Service.WithContext(ctx)
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
	return p.Service.DraftCompiler()
}

func (s *DashboardService) SetBrowserWorkerClient(client publisher.BrowserWorkerClient) {
	s.Publisher.SetBrowserWorkerClient(client)
}

func (s *DashboardService) SetBrowserSessionService(svc *browsersession.BrowserSessionService) {
	s.Publisher.SetBrowserSessionService(svc)
}

func (s *DashboardService) SetCollabDocumentService(svc *collabdoc.Service) {
	s.Project.Service.SetCollabDocumentService(svc)
}

func (s *DashboardService) SetDraftCompiler(compiler ProjectDraftCompiler) {
	s.Prepublish.SetDraftCompiler(compiler)
}

func NewDashboardServiceWithWechatTester(db *gorm.DB, tester platformaccount.WechatConnectionTester) *DashboardService {
	return NewDashboardServiceWithPlatformTesters(db, tester, platformaccount.XAPITester{})
}

func NewDashboardServiceWithPlatformTesters(db *gorm.DB, tester platformaccount.WechatConnectionTester, xTester platformaccount.XConnectionTester) *DashboardService {
	accounts := platformaccount.NewServiceWithPlatformTesters(db, tester, xTester)
	projects := projectsvc.NewService(db)
	publisher := publishsvc.NewService(db, accounts)
	return &DashboardService{
		Project:         &Project{Service: projects},
		Workspace:       &Workspace{Service: workspacesvc.NewService(db, projects)},
		Prepublish:      &Prepublish{Service: prepublishsvc.NewService(db, projects, compiler.NewContentPipelineDraftCompiler())},
		Extension:       &Extension{Service: extensionsvc.NewService(db)},
		Stats:           &Stats{Service: statssvc.NewService(db, projects)},
		AccountSettings: &AccountSettings{Service: accounts},
		Publisher:       &Publisher{Service: publisher},
		db:              db,
	}
}

func NewDashboardServiceWithXOAuth2Provider(db *gorm.DB, provider platformaccount.XOAuth2Provider) *DashboardService {
	accounts := platformaccount.NewServiceWithXOAuth2Provider(db, provider)
	projects := projectsvc.NewService(db)
	publisher := publishsvc.NewService(db, accounts)
	return &DashboardService{
		Project:         &Project{Service: projects},
		Workspace:       &Workspace{Service: workspacesvc.NewService(db, projects)},
		Prepublish:      &Prepublish{Service: prepublishsvc.NewService(db, projects, compiler.NewContentPipelineDraftCompiler())},
		Extension:       &Extension{Service: extensionsvc.NewService(db)},
		Stats:           &Stats{Service: statssvc.NewService(db, projects)},
		AccountSettings: &AccountSettings{Service: accounts},
		Publisher:       &Publisher{Service: publisher},
		db:              db,
	}
}

func (s *DashboardService) SetPublishQueue(queue publishsvc.PublishQueue) {
	s.Publisher.SetQueue(queue)
}

func (s *DashboardService) UseRedis(client *redis.Client) {
	if client == nil {
		return
	}
	s.AccountSettings.UseRedis(client)
	s.Publisher.UseRedis(client)
}

func (s *DashboardService) requestContext() context.Context {
	if s.db != nil && s.db.Statement != nil && s.db.Statement.Context != nil {
		return s.db.Statement.Context
	}
	return context.Background()
}

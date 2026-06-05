package dashboard

import (
	"context"
	"errors"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	collabdoc "github.com/kurodakayn/mpp-backend/internal/services/collabdoc"
	platformaccount "github.com/kurodakayn/mpp-backend/internal/services/platform_account"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var ErrForbidden = publishsvc.ErrForbidden
var ErrInvalidProject = errors.New("invalid project")
var ErrInvalidProjectCollaborator = errors.New("invalid project collaborator")
var ErrInvalidWorkspace = errors.New("invalid workspace")
var ErrInvalidWorkspaceMember = errors.New("invalid workspace member")
var ErrProjectCollabUnavailable = errors.New("project collaboration unavailable")
var ErrPublicationDisabled = publishsvc.ErrPublicationDisabled
var ErrPublicationRequiresSync = publishsvc.ErrPublicationRequiresSync
var ErrManualPublishUnsupported = publishsvc.ErrManualPublishUnsupported
var ErrExtensionCallbackTokenInvalid = errors.New("invalid extension callback token")
var ErrExtensionCallbackTokenExpired = errors.New("expired extension callback token")

var allowedProjectPlatforms = map[string]struct{}{
	"douyin": {},
	"wechat": {},
	"x":      {},
	"zhihu":  {},
}

type DashboardService struct {
	db                    *gorm.DB
	accounts              *platformaccount.Service
	publisher             *publishsvc.Service
	browserWorkerClient   publisher.BrowserWorkerClient
	browserSessionService *browsersession.BrowserSessionService
	collabDocuments       *collabdoc.Service
	draftCompiler         ProjectDraftCompiler
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
	if s.accounts != nil {
		scoped.accounts = s.accounts.WithContext(ctx)
	}
	if s.publisher != nil {
		scoped.publisher = s.publisher.WithContext(ctx)
	}
	if s.browserSessionService != nil {
		scoped.browserSessionService = s.browserSessionService.WithContext(ctx)
	}
	if s.collabDocuments != nil {
		scoped.collabDocuments = s.collabDocuments.WithContext(ctx)
	}
	return &scoped
}

func (s *DashboardService) SetBrowserWorkerClient(client publisher.BrowserWorkerClient) {
	s.browserWorkerClient = client
}

func (s *DashboardService) SetBrowserSessionService(svc *browsersession.BrowserSessionService) {
	s.browserSessionService = svc
}

func (s *DashboardService) SetCollabDocumentService(svc *collabdoc.Service) {
	s.collabDocuments = svc
}

func (s *DashboardService) SetDraftCompiler(compiler ProjectDraftCompiler) {
	s.draftCompiler = compiler
}

func NewDashboardServiceWithWechatTester(db *gorm.DB, tester platformaccount.WechatConnectionTester) *DashboardService {
	return NewDashboardServiceWithPlatformTesters(db, tester, platformaccount.XAPITester{})
}

func NewDashboardServiceWithPlatformTesters(db *gorm.DB, tester platformaccount.WechatConnectionTester, xTester platformaccount.XConnectionTester) *DashboardService {
	accounts := platformaccount.NewServiceWithPlatformTesters(db, tester, xTester)
	return &DashboardService{
		db:            db,
		accounts:      accounts,
		publisher:     publishsvc.NewService(db, accounts),
		draftCompiler: newContentPipelineDraftCompiler(),
	}
}

func NewDashboardServiceWithXOAuth2Provider(db *gorm.DB, provider platformaccount.XOAuth2Provider) *DashboardService {
	accounts := platformaccount.NewServiceWithXOAuth2Provider(db, provider)
	return &DashboardService{
		db:            db,
		accounts:      accounts,
		publisher:     publishsvc.NewService(db, accounts),
		draftCompiler: newContentPipelineDraftCompiler(),
	}
}

func (s *DashboardService) SetPublishQueue(queue publishsvc.PublishQueue) {
	s.publisher.SetQueue(queue)
}

func (s *DashboardService) UseRedis(client *redis.Client) {
	if client == nil {
		return
	}
	s.accounts.UseRedis(client)
	s.publisher.UseRedis(client)
	if s.browserSessionService != nil {
		s.browserSessionService.UseRedis(client)
	}
}

func (s *DashboardService) requestContext() context.Context {
	if s.db != nil && s.db.Statement != nil && s.db.Statement.Context != nil {
		return s.db.Statement.Context
	}
	return context.Background()
}

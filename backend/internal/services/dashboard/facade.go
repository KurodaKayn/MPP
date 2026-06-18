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
	readmodelsvc "github.com/kurodakayn/mpp-backend/internal/services/readmodel"
	statssvc "github.com/kurodakayn/mpp-backend/internal/services/stats"
	workspacesvc "github.com/kurodakayn/mpp-backend/internal/services/workspace"
)

type ProjectDraftCompiler = compiler.ProjectDraftCompiler

type DashboardService struct {
	*Project
	*Workspace
	*Prepublish
	*Extension
	*MediaAsset
	*Stats
	*AccountSettings
	*Publisher

	db                    *gorm.DB
	dbRouter              *dbrouter.Router
	readModel             *readmodelsvc.Service
	readModelRebuildQueue DashboardReadModelRebuildQueue
}

type DashboardReadModelRebuildQueue interface {
	EnqueueDashboardRebuild(ctx context.Context) (readmodelsvc.DashboardRebuildTaskInfo, error)
	StartWorker(ctx context.Context, service *readmodelsvc.Service) error
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
	draftCompiler := s.ServiceDraftCompiler()
	scoped := *s
	scoped.db = s.db.WithContext(ctx)
	scoped.Project = &Project{Service: s.Project.WithContext(ctx)}
	scoped.Workspace = &Workspace{Service: workspacesvc.NewServiceWithRouter(scoped.db, scoped.Project.Service, scoped.dbRouter)}
	scoped.Prepublish = &Prepublish{Service: prepublishsvc.NewServiceWithRouter(scoped.db, scoped.Project.Service, draftCompiler, scoped.dbRouter)}
	scoped.Extension = &Extension{Service: extensionsvc.NewServiceWithRouter(scoped.db, scoped.dbRouter)}
	scoped.MediaAsset = &MediaAsset{Service: s.MediaAsset.WithContext(ctx)}
	scoped.Stats = &Stats{Service: s.Stats.WithContext(ctx)}
	scoped.AccountSettings = &AccountSettings{Service: s.AccountSettings.WithContext(ctx)}
	scoped.Publisher = &Publisher{Service: s.Publisher.WithContext(ctx)}
	if s.readModel != nil {
		scoped.readModel = s.readModel.WithContext(ctx)
	}
	scopedService := &scoped
	scopedService.wireDashboardCacheInvalidators()
	return scopedService
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
	accounts := platformaccount.NewServiceWithPlatformTestersAndRouter(db, tester, xTester, router)
	projects := projectsvc.NewServiceWithRouter(db, router)
	publisher := publishsvc.NewServiceWithRouter(db, accounts, router)
	stats := statssvc.NewServiceWithRouter(db, projects, router)
	prepublish := prepublishsvc.NewServiceWithRouter(db, projects, compiler.NewContentPipelineDraftCompiler(), router)
	readModel := readmodelsvc.NewService(db)
	service := &DashboardService{
		Project:         &Project{Service: projects},
		Workspace:       &Workspace{Service: workspacesvc.NewServiceWithRouter(db, projects, router)},
		Prepublish:      &Prepublish{Service: prepublish},
		Extension:       &Extension{Service: extensionsvc.NewServiceWithRouter(db, router)},
		MediaAsset:      &MediaAsset{Service: mediaassetsvc.NewServiceWithRouter(db, projects, router)},
		Stats:           &Stats{Service: stats},
		AccountSettings: &AccountSettings{Service: accounts},
		Publisher:       &Publisher{Service: publisher},
		db:              db,
		dbRouter:        router,
		readModel:       readModel,
	}
	service.wireDashboardCacheInvalidators()
	return service
}

func NewDashboardServiceWithXOAuth2Provider(db *gorm.DB, provider platformaccount.XOAuth2Provider) *DashboardService {
	router := dbrouter.NewRouter(db)
	accounts := platformaccount.NewServiceWithXOAuth2ProviderAndRouter(db, provider, router)
	projects := projectsvc.NewServiceWithRouter(db, router)
	publisher := publishsvc.NewServiceWithRouter(db, accounts, router)
	stats := statssvc.NewServiceWithRouter(db, projects, router)
	prepublish := prepublishsvc.NewServiceWithRouter(db, projects, compiler.NewContentPipelineDraftCompiler(), router)
	readModel := readmodelsvc.NewService(db)
	service := &DashboardService{
		Project:         &Project{Service: projects},
		Workspace:       &Workspace{Service: workspacesvc.NewServiceWithRouter(db, projects, router)},
		Prepublish:      &Prepublish{Service: prepublish},
		Extension:       &Extension{Service: extensionsvc.NewServiceWithRouter(db, router)},
		MediaAsset:      &MediaAsset{Service: mediaassetsvc.NewServiceWithRouter(db, projects, router)},
		Stats:           &Stats{Service: stats},
		AccountSettings: &AccountSettings{Service: accounts},
		Publisher:       &Publisher{Service: publisher},
		db:              db,
		dbRouter:        router,
		readModel:       readModel,
	}
	service.wireDashboardCacheInvalidators()
	return service
}

func (s *DashboardService) SetPublishQueue(queue publishsvc.PublishQueue) {
	s.SetQueue(queue)
}

func (s *DashboardService) UseRedis(client *redis.Client) {
	s.UseRedisCache(client)
	s.UseRedisQueue(client)
}

func (s *DashboardService) UseRedisCache(client *redis.Client) {
	if client == nil {
		return
	}
	s.Project.UseRedisCache(client)
	s.AccountSettings.UseRedisCache(client)
	s.MediaAsset.UseRedisCache(client)
	s.Stats.UseRedisCache(client)
}

func (s *DashboardService) UseRedisQueue(client *redis.Client) {
	if client == nil {
		return
	}
	s.Publisher.UseRedisQueue(client)
	s.readModelRebuildQueue = readmodelsvc.NewRedisDashboardRebuildQueue(client)
}

func (s *DashboardService) EnqueueDashboardReadModelRebuild(ctx context.Context) (readmodelsvc.DashboardRebuildTaskInfo, error) {
	if s == nil || s.readModelRebuildQueue == nil {
		return readmodelsvc.DashboardRebuildTaskInfo{}, readmodelsvc.ErrDashboardRebuildQueueUnavailable
	}
	return s.readModelRebuildQueue.EnqueueDashboardRebuild(ctx)
}

func (s *DashboardService) StartDashboardReadModelRebuildWorkerWithErrors(ctx context.Context) <-chan error {
	if s == nil || s.readModelRebuildQueue == nil || s.readModel == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	workerErrors := make(chan error, 1)
	go func() {
		defer close(workerErrors)
		if err := s.readModelRebuildQueue.StartWorker(ctx, s.readModel); err != nil && ctx.Err() == nil {
			workerErrors <- err
		}
	}()
	return workerErrors
}

func (s *DashboardService) wireDashboardCacheInvalidators() {
	if s == nil {
		return
	}
	sideEffects := s.dashboardSideEffects()
	if s.Project != nil && s.Project.Service != nil && s.Stats != nil && s.Stats.Service != nil {
		s.Project.SetDashboardStatsCacheInvalidator(sideEffects)
	}
	if s.Project != nil && s.Project.Service != nil {
		s.Project.SetDashboardReadModelUpdater(sideEffects)
	}
	if s.Prepublish != nil && s.Prepublish.Service != nil && s.Stats != nil && s.Stats.Service != nil {
		s.Prepublish.SetDashboardStatsCacheInvalidator(sideEffects)
	}
	if s.Prepublish != nil && s.Prepublish.Service != nil {
		s.Prepublish.SetDashboardReadModelUpdater(sideEffects)
	}
	if s.Workspace != nil && s.Workspace.Service != nil {
		s.Workspace.SetDashboardReadModelUpdater(sideEffects)
		s.SetDashboardProjectListCacheInvalidator(sideEffects)
		s.Workspace.SetDashboardStatsCacheInvalidator(sideEffects)
	}
	if s.Publisher != nil && s.Publisher.Service != nil {
		publisher := s.Publisher.Service
		publisher.SetDashboardCacheInvalidator(sideEffects)
		publisher.SetDashboardReadModelUpdater(sideEffects)
	}
}

func (s *DashboardService) dashboardSideEffects() dashboardSideEffects {
	var projectLists dashboardProjectListInvalidator
	if s.Project != nil && s.Project.Service != nil {
		projectLists = s.Project.Service
	}
	var stats dashboardStatsInvalidator
	if s.Stats != nil && s.Stats.Service != nil {
		stats = s.Stats.Service
	}
	return dashboardSideEffects{
		projectLists: projectLists,
		stats:        stats,
		readModels:   s.readModel,
	}
}

func (s *DashboardService) InvalidateDashboardProjectListCache(ctx context.Context) {
	if s == nil {
		return
	}
	s.dashboardSideEffects().InvalidateDashboardProjectListCache(ctx)
}

func (s *DashboardService) InvalidateDashboardStatsCache(ctx context.Context) {
	if s == nil {
		return
	}
	s.dashboardSideEffects().InvalidateDashboardStatsCache(ctx)
}

func (s *DashboardService) InvalidateDashboardScopedStatsCache(ctx context.Context) {
	if s == nil {
		return
	}
	s.dashboardSideEffects().InvalidateDashboardScopedStatsCache(ctx)
}

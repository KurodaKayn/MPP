package platformaccount

import (
	"context"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

type Service struct {
	db              *gorm.DB
	wechatTester    WechatConnectionTester
	xTester         XConnectionTester
	xOAuth2Provider XOAuth2Provider
	xOAuth2States   XOAuth2StateStore
}

func NewService(db *gorm.DB) *Service {
	return NewServiceWithPlatformTesters(db, WechatAPITester{}, XAPITester{})
}

func NewServiceWithWechatTester(db *gorm.DB, tester WechatConnectionTester) *Service {
	return NewServiceWithPlatformTesters(db, tester, XAPITester{})
}

func NewServiceWithPlatformTesters(db *gorm.DB, tester WechatConnectionTester, xTester XConnectionTester) *Service {
	if tester == nil {
		tester = WechatAPITester{}
	}
	if xTester == nil {
		xTester = XAPITester{}
	}
	return &Service{
		db:              db,
		wechatTester:    tester,
		xTester:         xTester,
		xOAuth2Provider: XOAuth2API{},
		xOAuth2States:   NewMemoryXOAuth2StateStore(),
	}
}

func NewServiceWithXOAuth2Provider(db *gorm.DB, provider XOAuth2Provider) *Service {
	service := NewService(db)
	if provider != nil {
		service.xOAuth2Provider = provider
	}
	return service
}

func (s *Service) WithContext(ctx context.Context) *Service {
	if ctx == nil {
		return s
	}
	scoped := *s
	scoped.db = s.db.WithContext(ctx)
	return &scoped
}

func (s *Service) UseRedis(client *redis.Client) {
	if client == nil {
		return
	}
	s.xOAuth2States = NewRedisXOAuth2StateStore(client)
}

func (s *Service) ApplySavedCredentialsToPublication(userID uuid.UUID, pub *models.ProjectPlatformPublication) error {
	if err := s.applySavedWechatCredentialsToPublication(userID, pub); err != nil {
		return err
	}
	if err := s.applySavedXCredentialsToPublication(userID, pub); err != nil {
		return err
	}
	return nil
}

package platformaccount

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
)

var ErrPlatformAccountForbidden = errors.New("platform account access denied")

type Service struct {
	db              *gorm.DB
	router          *dbrouter.Router
	wechatTester    WechatConnectionTester
	xTester         XConnectionTester
	xOAuth2Provider XOAuth2Provider
	xOAuth2States   XOAuth2StateStore
	cache           *redis.Client
	cacheTTL        time.Duration
	cacheGroup      *singleflight.Group
}

const dashboardAccountCacheTTL = 15 * time.Second

func NewService(db *gorm.DB) *Service {
	return NewServiceWithPlatformTesters(db, WechatAPITester{}, XAPITester{})
}

func NewServiceWithRouter(db *gorm.DB, router *dbrouter.Router) *Service {
	return NewServiceWithPlatformTestersAndRouter(db, WechatAPITester{}, XAPITester{}, router)
}

func NewServiceWithWechatTester(db *gorm.DB, tester WechatConnectionTester) *Service {
	return NewServiceWithPlatformTesters(db, tester, XAPITester{})
}

func NewServiceWithPlatformTesters(db *gorm.DB, tester WechatConnectionTester, xTester XConnectionTester) *Service {
	return NewServiceWithPlatformTestersAndRouter(db, tester, xTester, nil)
}

func NewServiceWithPlatformTestersAndRouter(db *gorm.DB, tester WechatConnectionTester, xTester XConnectionTester, router *dbrouter.Router) *Service {
	if tester == nil {
		tester = WechatAPITester{}
	}
	if xTester == nil {
		xTester = XAPITester{}
	}
	if router == nil {
		router = dbrouter.NewRouter(db)
	}
	return &Service{
		db:              db,
		router:          router,
		wechatTester:    tester,
		xTester:         xTester,
		xOAuth2Provider: XOAuth2API{},
		xOAuth2States:   NewMemoryXOAuth2StateStore(),
		cacheGroup:      &singleflight.Group{},
	}
}

func NewServiceWithXOAuth2Provider(db *gorm.DB, provider XOAuth2Provider) *Service {
	return NewServiceWithXOAuth2ProviderAndRouter(db, provider, nil)
}

func NewServiceWithXOAuth2ProviderAndRouter(db *gorm.DB, provider XOAuth2Provider, router *dbrouter.Router) *Service {
	service := NewServiceWithRouter(db, router)
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
	scoped.cacheGroup = s.cacheGroup
	return &scoped
}

func (s *Service) requestContext() context.Context {
	if s.db != nil && s.db.Statement != nil && s.db.Statement.Context != nil {
		return s.db.Statement.Context
	}
	return context.Background()
}

func (s *Service) strongReadDB() *gorm.DB {
	if s.router == nil {
		return s.db
	}
	return s.router.Reader(s.requestContext(), dbrouter.StrongRead)
}

func (s *Service) UseRedis(client *redis.Client) {
	if client == nil {
		return
	}
	s.xOAuth2States = NewRedisXOAuth2StateStore(client)
	s.cache = client
	s.cacheTTL = dashboardAccountCacheTTL
	if s.cacheGroup == nil {
		s.cacheGroup = &singleflight.Group{}
	}
}

func (s *Service) ApplySavedCredentialsToPublication(userID uuid.UUID, pub *models.ProjectPlatformPublication) error {
	account, err := s.ResolvePublicationAccount(userID, pub)
	if err != nil {
		if errors.Is(err, ErrInvalidPlatformAccount) && publicationHasEmbeddedCredentials(pub) {
			return nil
		}
		return err
	}
	if err := s.applySavedWechatCredentialsToPublication(account, pub); err != nil {
		return err
	}
	if err := s.applySavedXCredentialsToPublication(account, pub); err != nil {
		return err
	}
	return nil
}

func publicationHasEmbeddedCredentials(pub *models.ProjectPlatformPublication) bool {
	if pub == nil || len(pub.Config) == 0 {
		return false
	}
	config := map[string]string{}
	if err := json.Unmarshal(pub.Config, &config); err != nil {
		return false
	}
	switch pub.Platform {
	case wechatPlatform:
		return strings.TrimSpace(config["app_id"]) != "" && strings.TrimSpace(config["app_secret"]) != ""
	case xPlatform:
		return strings.TrimSpace(config["api_key"]) != "" &&
			strings.TrimSpace(config["api_secret"]) != "" &&
			strings.TrimSpace(config["access_token"]) != "" &&
			strings.TrimSpace(config["access_token_secret"]) != ""
	default:
		return false
	}
}

func (s *Service) WorkspaceIDForUser(userID uuid.UUID, workspaceID uuid.UUID) uuid.UUID {
	if workspaceID != uuid.Nil {
		return workspaceID
	}
	return models.PersonalWorkspaceID(userID)
}

func (s *Service) ResolvePublicationAccount(userID uuid.UUID, pub *models.ProjectPlatformPublication) (*models.PlatformAccount, error) {
	if pub == nil {
		return nil, ErrInvalidPlatformAccount
	}
	workspaceID := uuid.Nil
	readDB := s.strongReadDB()
	var project models.Project
	if err := readDB.Select("workspace_id", "user_id").First(&project, "id = ?", pub.ProjectID).Error; err == nil {
		if project.WorkspaceID != nil {
			workspaceID = *project.WorkspaceID
		} else {
			workspaceID = models.PersonalWorkspaceID(project.UserID)
		}
	}
	if workspaceID == uuid.Nil {
		workspaceID = models.PersonalWorkspaceID(userID)
	}

	var account models.PlatformAccount
	query := readDB.Where("workspace_id = ? AND platform = ?", workspaceID, pub.Platform)
	if pub.PlatformAccountID != nil && *pub.PlatformAccountID != uuid.Nil {
		query = query.Where("id = ?", *pub.PlatformAccountID)
		if err := query.First(&account).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrInvalidPlatformAccount
			}
			return nil, err
		}
		if err := s.RequireAccountUse(userID, account, pub.ProjectID); err != nil {
			return nil, err
		}
		if !accountReadyForPublication(account) {
			if err := s.recordAccountNeedsReauthNotification(userID, workspaceID, account, pub.ProjectID); err != nil {
				return nil, err
			}
			return nil, ErrInvalidPlatformAccount
		}
		return &account, nil
	}

	var accounts []models.PlatformAccount
	if err := query.Order("updated_at DESC").Find(&accounts).Error; err != nil {
		return nil, err
	}
	var firstUnavailable *models.PlatformAccount
	for i := range accounts {
		account = accounts[i]
		if err := s.RequireAccountUse(userID, account, pub.ProjectID); err != nil {
			if errors.Is(err, ErrPlatformAccountForbidden) {
				continue
			}
			return nil, err
		}
		if !accountReadyForPublication(account) {
			if firstUnavailable == nil {
				firstUnavailable = &accounts[i]
			}
			continue
		}
		accountID := account.ID
		pub.PlatformAccountID = &accountID
		return &account, nil
	}
	if firstUnavailable != nil {
		if err := s.recordAccountNeedsReauthNotification(userID, workspaceID, *firstUnavailable, pub.ProjectID); err != nil {
			return nil, err
		}
		return nil, ErrInvalidPlatformAccount
	}
	if len(accounts) > 0 {
		return nil, ErrPlatformAccountForbidden
	}
	return nil, ErrInvalidPlatformAccount
}

func (s *Service) recordAccountNeedsReauthNotification(userID uuid.UUID, workspaceID uuid.UUID, account models.PlatformAccount, projectID uuid.UUID) error {
	if userID == uuid.Nil || workspaceID == uuid.Nil || account.ID == uuid.Nil {
		return nil
	}
	payload := map[string]any{
		"platform":            account.Platform,
		"platform_account_id": account.ID.String(),
		"status":              account.Status,
		"health_status":       account.HealthStatus,
	}
	if projectID != uuid.Nil {
		payload["project_id"] = projectID.String()
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resourceID := account.ID
	return s.db.Create(&models.Notification{
		WorkspaceID:     workspaceID,
		RecipientUserID: userID,
		EventType:       models.NotificationAccountNeedsReauth,
		ResourceType:    "platform_account",
		ResourceID:      &resourceID,
		Status:          models.NotificationStatusUnread,
		Metadata:        datatypes.JSON(encoded),
	}).Error
}

func (s *Service) RequireAccountUse(userID uuid.UUID, account models.PlatformAccount, projectID uuid.UUID) error {
	if account.WorkspaceID != nil && (account.OwnerUserID != nil && *account.OwnerUserID == userID || account.UserID == userID) {
		if err := s.requireWorkspaceAccountMembership(userID, *account.WorkspaceID); err != nil {
			return err
		}
		return nil
	}
	if account.ShareScope == models.PlatformAccountShareWorkspace {
		if account.WorkspaceID == nil {
			return ErrPlatformAccountForbidden
		}
		_, err := accesspolicy.RequireWorkspacePermissionWithDB(s.strongReadDB(), *account.WorkspaceID, userID, accesspolicy.PermissionAccountUse)
		if err == nil {
			return nil
		}
		if !errors.Is(err, accesspolicy.ErrForbidden) && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	roles := []string{models.PlatformAccountGrantRoleManager, models.PlatformAccountGrantRolePublisher}
	readDB := s.strongReadDB()
	var grant models.PlatformAccountGrant
	if err := readDB.Select("id").
		Where("platform_account_id = ? AND grantee_user_id = ? AND role IN ?", account.ID, userID, roles).
		First(&grant).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if projectID != uuid.Nil {
		if err := readDB.Select("id").
			Where("platform_account_id = ? AND project_id = ? AND role IN ?", account.ID, projectID, roles).
			First(&grant).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	return ErrPlatformAccountForbidden
}

func (s *Service) requireWorkspaceAccountMembership(userID uuid.UUID, workspaceID uuid.UUID) error {
	if err := accesspolicy.RequireWorkspaceMemberRecordWithDB(s.strongReadDB(), workspaceID, userID); err != nil {
		if errors.Is(err, accesspolicy.ErrForbidden) || errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPlatformAccountForbidden
		}
		return err
	}
	return nil
}

func (s *Service) ensureAccountDefaults(account *models.PlatformAccount, userID uuid.UUID, workspaceID uuid.UUID, platform string) {
	if account.UserID == uuid.Nil {
		account.UserID = userID
	}
	if account.WorkspaceID == nil {
		id := s.WorkspaceIDForUser(userID, workspaceID)
		account.WorkspaceID = &id
	}
	if account.OwnerUserID == nil {
		id := userID
		account.OwnerUserID = &id
	}
	if account.ConnectedByUserID == nil {
		id := userID
		account.ConnectedByUserID = &id
	}
	if strings.TrimSpace(account.Platform) == "" {
		account.Platform = platform
	}
	if strings.TrimSpace(account.ShareScope) == "" {
		account.ShareScope = models.PlatformAccountSharePrivate
	}
	if strings.TrimSpace(account.HealthStatus) == "" {
		account.HealthStatus = healthStatusForStatus(account.Status)
	}
	if strings.TrimSpace(account.CredentialSecretRef) == "" && account.ID != uuid.Nil {
		account.CredentialSecretRef = "platform-account:" + account.ID.String()
	}
	if account.Metadata == nil {
		account.Metadata = datatypes.JSON([]byte(`{}`))
	}
	if account.Credentials == nil {
		account.Credentials = datatypes.JSON([]byte(`{}`))
	}
	if account.Cookies == nil {
		account.Cookies = datatypes.JSON([]byte(`[]`))
	}
	if account.Config == nil {
		account.Config = datatypes.JSON([]byte(`{}`))
	}
}

func healthStatusForStatus(status string) string {
	switch status {
	case models.PlatformAccountStatusConnected:
		return models.PlatformAccountHealthHealthy
	case models.PlatformAccountStatusFailed:
		return models.PlatformAccountHealthFailed
	case models.PlatformAccountStatusNeedsReauth:
		return models.PlatformAccountHealthNeedsReauth
	default:
		return models.PlatformAccountHealthUnknown
	}
}

func accountReadyForPublication(account models.PlatformAccount) bool {
	return account.Status == models.PlatformAccountStatusConnected &&
		account.HealthStatus != models.PlatformAccountHealthNeedsReauth &&
		account.HealthStatus != models.PlatformAccountHealthFailed
}

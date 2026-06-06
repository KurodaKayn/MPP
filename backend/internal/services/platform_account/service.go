package platformaccount

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

var ErrPlatformAccountForbidden = errors.New("platform account access denied")

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
	account, err := s.ResolvePublicationAccount(userID, pub)
	if err != nil {
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
	var project models.Project
	if err := s.db.Select("workspace_id", "user_id").First(&project, "id = ?", pub.ProjectID).Error; err == nil {
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
	query := s.db.Where("workspace_id = ? AND platform = ?", workspaceID, pub.Platform)
	if pub.PlatformAccountID != nil && *pub.PlatformAccountID != uuid.Nil {
		query = query.Where("id = ?", *pub.PlatformAccountID)
	} else {
		query = query.Order("updated_at DESC")
	}
	if err := query.First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidPlatformAccount
		}
		return nil, err
	}
	if err := s.RequireAccountUse(userID, account, pub.ProjectID); err != nil {
		return nil, err
	}
	if account.Status != models.PlatformAccountStatusConnected || account.HealthStatus == models.PlatformAccountHealthNeedsReauth || account.HealthStatus == models.PlatformAccountHealthFailed {
		if err := s.recordAccountNeedsReauthNotification(userID, workspaceID, account, pub.ProjectID); err != nil {
			return nil, err
		}
		return nil, ErrInvalidPlatformAccount
	}
	if pub.PlatformAccountID == nil || *pub.PlatformAccountID == uuid.Nil {
		accountID := account.ID
		pub.PlatformAccountID = &accountID
	}
	return &account, nil
}

func (s *Service) recordAccountNeedsReauthNotification(userID uuid.UUID, workspaceID uuid.UUID, account models.PlatformAccount, projectID uuid.UUID) error {
	if userID == uuid.Nil || workspaceID == uuid.Nil || account.ID == uuid.Nil {
		return nil
	}
	metadata := datatypes.JSON([]byte(`{}`))
	payload := map[string]any{
		"platform":            account.Platform,
		"platform_account_id": account.ID.String(),
		"status":              account.Status,
		"health_status":       account.HealthStatus,
	}
	if projectID != uuid.Nil {
		payload["project_id"] = projectID.String()
	}
	if encoded, err := json.Marshal(payload); err == nil {
		metadata = datatypes.JSON(encoded)
	} else {
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
		Metadata:        metadata,
	}).Error
}

func (s *Service) RequireAccountUse(userID uuid.UUID, account models.PlatformAccount, projectID uuid.UUID) error {
	if account.OwnerUserID != nil && *account.OwnerUserID == userID {
		return nil
	}
	if account.UserID == userID {
		return nil
	}
	if account.ShareScope == models.PlatformAccountShareWorkspace {
		if account.WorkspaceID == nil {
			return ErrPlatformAccountForbidden
		}
		var member models.WorkspaceMember
		err := s.db.Select("role").First(&member, "workspace_id = ? AND user_id = ?", *account.WorkspaceID, userID).Error
		if err == nil && member.Role != models.WorkspaceRoleViewer {
			return nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	roles := []string{models.PlatformAccountGrantRoleManager, models.PlatformAccountGrantRolePublisher}
	var grant models.PlatformAccountGrant
	if err := s.db.Select("id").
		Where("platform_account_id = ? AND grantee_user_id = ? AND role IN ?", account.ID, userID, roles).
		First(&grant).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if projectID != uuid.Nil {
		if err := s.db.Select("id").
			Where("platform_account_id = ? AND project_id = ? AND role IN ?", account.ID, projectID, roles).
			First(&grant).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	return ErrPlatformAccountForbidden
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

func accountConnectedUpdates(userID uuid.UUID, status string) map[string]any {
	now := time.Now().UTC()
	return map[string]any{
		"connected_by_user_id": userID,
		"last_connected_at":    &now,
		"last_verified_at":     &now,
		"health_status":        healthStatusForStatus(status),
		"status":               status,
	}
}

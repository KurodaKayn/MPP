package collabdoc

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

var (
	ErrInvalidDocument   = errors.New("invalid collaborative document")
	ErrDocumentForbidden = errors.New("collaborative document forbidden")
)

const (
	defaultDocumentListLimit = 20
	maxDocumentListLimit     = 100
	defaultSessionTTL        = 5 * time.Minute
	defaultHeartbeatSeconds  = 30
	defaultMaxMessageBytes   = 512 * 1024
	defaultWebsocketURLBase  = "ws://localhost:8090"
)

type Service struct {
	db                         *gorm.DB
	sessionConfig              SessionConfig
	projectDocumentInitializer ProjectDocumentInitializer
}

type DocumentList struct {
	Items      []models.CollabDocument
	Page       int
	Limit      int
	Total      int64
	TotalPages int
}

type SessionConfig struct {
	TokenSecret      []byte
	WebsocketURLBase string
	TTL              time.Duration
	MaxMessageBytes  int
	HeartbeatSeconds int
}

type SessionLimits struct {
	MaxMessageBytes  int
	HeartbeatSeconds int
}

type Session struct {
	DocumentID   uuid.UUID
	Role         string
	WebsocketURL string
	Token        string
	ExpiresAt    time.Time
	Limits       SessionLimits
}

type sessionClaims struct {
	UserID     uuid.UUID `json:"user_id"`
	DocumentID uuid.UUID `json:"document_id"`
	Role       string    `json:"role"`
	Purpose    string    `json:"purpose"`
	jwt.RegisteredClaims
}

func NewService(db *gorm.DB) *Service {
	return &Service{
		db: db,
		sessionConfig: SessionConfig{
			WebsocketURLBase: defaultWebsocketURLBase,
			TTL:              defaultSessionTTL,
			MaxMessageBytes:  defaultMaxMessageBytes,
			HeartbeatSeconds: defaultHeartbeatSeconds,
		},
	}
}

func (s *Service) UseSessionConfig(config SessionConfig) {
	if len(config.TokenSecret) > 0 {
		s.sessionConfig.TokenSecret = config.TokenSecret
	}
	if strings.TrimSpace(config.WebsocketURLBase) != "" {
		s.sessionConfig.WebsocketURLBase = strings.TrimRight(strings.TrimSpace(config.WebsocketURLBase), "/")
	}
	if config.TTL > 0 {
		s.sessionConfig.TTL = config.TTL
	}
	if config.MaxMessageBytes > 0 {
		s.sessionConfig.MaxMessageBytes = config.MaxMessageBytes
	}
	if config.HeartbeatSeconds > 0 {
		s.sessionConfig.HeartbeatSeconds = config.HeartbeatSeconds
	}
}

func (s *Service) UseProjectDocumentInitializer(initializer ProjectDocumentInitializer) {
	s.projectDocumentInitializer = initializer
}

func (s *Service) WithContext(ctx context.Context) *Service {
	if ctx == nil {
		return s
	}
	scoped := *s
	scoped.db = s.db.WithContext(ctx)
	return &scoped
}

func (s *Service) CreateDocument(ctx context.Context, ownerUserID uuid.UUID, title string) (*models.CollabDocument, error) {
	title = strings.TrimSpace(title)
	if ownerUserID == uuid.Nil || title == "" {
		return nil, ErrInvalidDocument
	}

	document := models.CollabDocument{
		OwnerUserID:   ownerUserID,
		Title:         title,
		Status:        models.CollabDocumentStatusActive,
		SchemaVersion: 1,
		CurrentSeq:    0,
	}

	if err := s.WithContext(ctx).db.Create(&document).Error; err != nil {
		return nil, err
	}
	return &document, nil
}

func (s *Service) ListDocuments(ctx context.Context, userID uuid.UUID, page, limit int) (*DocumentList, error) {
	if userID == uuid.Nil {
		return nil, ErrInvalidDocument
	}

	page, limit = normalizePagination(page, limit)
	db := s.WithContext(ctx).db
	collaboratorDocumentIDs := db.
		Model(&models.CollabDocumentCollaborator{}).
		Select("document_id").
		Where("user_id = ?", userID)
	query := db.
		Model(&models.CollabDocument{}).
		Where("owner_user_id = ? OR id IN (?)", userID, collaboratorDocumentIDs)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	var documents []models.CollabDocument
	if err := query.
		Order("updated_at DESC").
		Order("id ASC").
		Limit(limit).
		Offset((page - 1) * limit).
		Find(&documents).Error; err != nil {
		return nil, err
	}

	return &DocumentList{
		Items:      documents,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages(total, limit),
	}, nil
}

func (s *Service) GetDocument(ctx context.Context, userID uuid.UUID, documentID uuid.UUID) (*models.CollabDocument, error) {
	if userID == uuid.Nil || documentID == uuid.Nil {
		return nil, ErrInvalidDocument
	}

	document, _, err := s.getAccessibleDocument(ctx, userID, documentID)
	return document, err
}

func (s *Service) CreateSession(ctx context.Context, userID uuid.UUID, documentID uuid.UUID) (*Session, error) {
	if userID == uuid.Nil || documentID == uuid.Nil {
		return nil, ErrInvalidDocument
	}

	document, role, err := s.getAccessibleDocument(ctx, userID, documentID)
	if err != nil {
		return nil, err
	}

	return s.createSession(userID, document.ID, role)
}

func (s *Service) InitializeProjectDocument(ctx context.Context, documentID uuid.UUID) error {
	if documentID == uuid.Nil || s.projectDocumentInitializer == nil {
		return ErrProjectDocumentInitialization
	}
	return s.projectDocumentInitializer.InitializeProjectDocument(ctx, documentID)
}

func (s *Service) SyncProjectSourceContent(ctx context.Context, documentID uuid.UUID) error {
	if documentID == uuid.Nil || s.projectDocumentInitializer == nil {
		return ErrProjectSourceContentSync
	}
	return s.projectDocumentInitializer.SyncProjectSourceContent(ctx, documentID)
}

// CreateAuthorizedSession issues a session after the caller has resolved access.
func (s *Service) CreateAuthorizedSession(ctx context.Context, userID uuid.UUID, documentID uuid.UUID, role string) (*Session, error) {
	if userID == uuid.Nil || documentID == uuid.Nil {
		return nil, ErrInvalidDocument
	}

	var document models.CollabDocument
	if err := s.WithContext(ctx).db.Select("id").First(&document, "id = ?", documentID).Error; err != nil {
		return nil, err
	}

	return s.createSession(userID, document.ID, role)
}

func (s *Service) createSession(userID uuid.UUID, documentID uuid.UUID, role string) (*Session, error) {
	if len(s.sessionConfig.TokenSecret) == 0 || !isCollabDocumentRole(role) {
		return nil, ErrInvalidDocument
	}

	now := time.Now().UTC()
	expiresAt := now.Add(s.sessionConfig.TTL)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, sessionClaims{
		UserID:     userID,
		DocumentID: documentID,
		Role:       role,
		Purpose:    "collab-session",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Audience:  []string{"mpp-collab-service"},
			Issuer:    "mpp-backend",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}).SignedString(s.sessionConfig.TokenSecret)
	if err != nil {
		return nil, err
	}

	return &Session{
		DocumentID:   documentID,
		Role:         role,
		WebsocketURL: s.sessionConfig.WebsocketURLBase + "/collab/documents/" + documentID.String(),
		Token:        token,
		ExpiresAt:    expiresAt,
		Limits: SessionLimits{
			MaxMessageBytes:  s.sessionConfig.MaxMessageBytes,
			HeartbeatSeconds: s.sessionConfig.HeartbeatSeconds,
		},
	}, nil
}

func isCollabDocumentRole(role string) bool {
	return role == models.CollabDocumentRoleEditor || role == models.CollabDocumentRoleViewer
}

func (s *Service) getAccessibleDocument(ctx context.Context, userID uuid.UUID, documentID uuid.UUID) (*models.CollabDocument, string, error) {
	db := s.WithContext(ctx).db
	var document models.CollabDocument
	if err := db.First(&document, "id = ?", documentID).Error; err != nil {
		return nil, "", err
	}
	if document.OwnerUserID == userID {
		return &document, models.CollabDocumentRoleEditor, nil
	}

	var collaborator models.CollabDocumentCollaborator
	if err := db.
		Model(&models.CollabDocumentCollaborator{}).
		Where("document_id = ? AND user_id = ?", documentID, userID).
		First(&collaborator).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrDocumentForbidden
		}
		return nil, "", err
	}
	return &document, collaborator.Role, nil
}

func (s *Service) UpdateDocumentTitle(ctx context.Context, userID uuid.UUID, documentID uuid.UUID, title string) (*models.CollabDocument, error) {
	title = strings.TrimSpace(title)
	if userID == uuid.Nil || documentID == uuid.Nil || title == "" {
		return nil, ErrInvalidDocument
	}

	db := s.WithContext(ctx).db
	var document models.CollabDocument
	if err := db.First(&document, "id = ?", documentID).Error; err != nil {
		return nil, err
	}
	if document.OwnerUserID != userID {
		return nil, ErrDocumentForbidden
	}

	if err := db.Model(&document).Update("title", title).Error; err != nil {
		return nil, err
	}

	document.Title = title
	return &document, nil
}

func normalizePagination(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = defaultDocumentListLimit
	}
	if limit > maxDocumentListLimit {
		limit = maxDocumentListLimit
	}
	return page, limit
}

func totalPages(total int64, limit int) int {
	if total == 0 {
		return 0
	}
	return int((total + int64(limit) - 1) / int64(limit))
}

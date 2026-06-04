package collabdoc

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"gorm.io/gorm"
)

var (
	ErrInvalidDocument   = errors.New("invalid collaborative document")
	ErrDocumentForbidden = errors.New("collaborative document forbidden")
)

const (
	defaultDocumentListLimit = 20
	maxDocumentListLimit     = 100
)

type Service struct {
	db *gorm.DB
}

type DocumentList struct {
	Items      []models.CollabDocument
	Page       int
	Limit      int
	Total      int64
	TotalPages int
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
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

	db := s.WithContext(ctx).db
	var document models.CollabDocument
	if err := db.First(&document, "id = ?", documentID).Error; err != nil {
		return nil, err
	}
	if document.OwnerUserID == userID {
		return &document, nil
	}

	var collaboratorCount int64
	if err := db.
		Model(&models.CollabDocumentCollaborator{}).
		Where("document_id = ? AND user_id = ?", documentID, userID).
		Count(&collaboratorCount).Error; err != nil {
		return nil, err
	}
	if collaboratorCount == 0 {
		return nil, ErrDocumentForbidden
	}

	return &document, nil
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

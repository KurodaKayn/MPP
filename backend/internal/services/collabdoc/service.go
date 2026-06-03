package collabdoc

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"gorm.io/gorm"
)

var ErrInvalidDocument = errors.New("invalid collaborative document")

type Service struct {
	db *gorm.DB
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

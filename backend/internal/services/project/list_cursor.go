package project

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

const (
	defaultProjectListLimit = 10
	maxProjectListLimit     = 100
)

var errEmptyProjectListCursor = errors.New("empty project list cursor")

type projectListCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

func normalizeProjectListPage(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = defaultProjectListLimit
	}
	if limit > maxProjectListLimit {
		limit = maxProjectListLimit
	}
	return page, limit
}

func applyProjectListCursor(query *gorm.DB, cursor string) (*gorm.DB, error) {
	return applyProjectListCursorColumns(query, cursor, "projects.created_at", "projects.id")
}

func applyProjectListCursorColumns(query *gorm.DB, cursor, createdAtColumn, idColumn string) (*gorm.DB, error) {
	if strings.TrimSpace(cursor) == "" {
		return query, nil
	}
	decoded, err := decodeProjectListCursor(cursor)
	if err != nil {
		return nil, err
	}
	return query.Where(
		fmt.Sprintf("(%s < ? OR (%s = ? AND %s > ?))", createdAtColumn, createdAtColumn, idColumn),
		decoded.CreatedAt,
		decoded.CreatedAt,
		decoded.ID,
	), nil
}

func decodeProjectListCursor(cursor string) (*projectListCursor, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return nil, errEmptyProjectListCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid project list cursor", ErrInvalidProject)
	}
	var decoded projectListCursor
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("%w: invalid project list cursor", ErrInvalidProject)
	}
	if decoded.ID == uuid.Nil || decoded.CreatedAt.IsZero() {
		return nil, fmt.Errorf("%w: invalid project list cursor", ErrInvalidProject)
	}
	return &decoded, nil
}

func encodeProjectListCursor(project models.Project) string {
	return encodeProjectListCursorValues(project.CreatedAt, project.ID)
}

func encodeProjectListCursorValues(createdAt time.Time, id uuid.UUID) string {
	encoded, err := json.Marshal(projectListCursor{
		CreatedAt: createdAt,
		ID:        id,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(encoded)
}

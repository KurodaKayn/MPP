package listing

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/project/projecterr"
)

const (
	defaultProjectListLimit = 10
	maxProjectListLimit     = 100
)

var errEmptyCursor = errors.New("empty project list cursor")

type cursorPayload struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

func NormalizePage(page, limit int) (int, int) {
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

func ApplyCursor(query *gorm.DB, cursor string) (*gorm.DB, error) {
	return ApplyCursorColumns(query, cursor, "projects.created_at", "projects.id")
}

func ApplyCursorColumns(query *gorm.DB, cursor, createdAtColumn, idColumn string) (*gorm.DB, error) {
	if strings.TrimSpace(cursor) == "" {
		return query, nil
	}
	decoded, err := decodeCursor(cursor)
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

func EncodeCursor(project models.Project) string {
	return EncodeCursorValues(project.CreatedAt, project.ID)
}

func EncodeCursorValues(createdAt time.Time, id uuid.UUID) string {
	encoded, err := json.Marshal(cursorPayload{
		CreatedAt: createdAt,
		ID:        id,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func PaginationResponse(items []dto.ProjectListItem, cursor string, page int, limit int, hasMore bool, nextCursor string) *dto.PaginationResponse {
	total := int64((page-1)*limit + len(items))
	if hasMore {
		total++
	}
	totalPages := page
	if len(items) == 0 && page == 1 {
		totalPages = 0
	} else if hasMore {
		totalPages = page + 1
	}

	return &dto.PaginationResponse{
		Items:      items,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
		Cursor:     strings.TrimSpace(cursor),
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}
}

func decodeCursor(cursor string) (*cursorPayload, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return nil, errEmptyCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid project list cursor", projecterr.ErrInvalidProject)
	}
	var decoded cursorPayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("%w: invalid project list cursor", projecterr.ErrInvalidProject)
	}
	if decoded.ID == uuid.Nil || decoded.CreatedAt.IsZero() {
		return nil, fmt.Errorf("%w: invalid project list cursor", projecterr.ErrInvalidProject)
	}
	return &decoded, nil
}

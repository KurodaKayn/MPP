package experience

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/accesspolicy"
)

func (s *Service) ListProjectComments(projectID uuid.UUID, userID uuid.UUID) (*dto.ProjectCommentsResponse, error) {
	if err := s.requireProjectAccess(projectID, userID); err != nil {
		return nil, err
	}

	var comments []models.ProjectComment
	if err := s.db.
		Preload("Author", selectUserIdentity).
		Where("project_id = ?", projectID).
		Order("created_at desc").
		Find(&comments).Error; err != nil {
		return nil, err
	}

	items := make([]dto.ProjectComment, 0, len(comments))
	for _, comment := range comments {
		items = append(items, projectCommentFromModel(comment))
	}
	return &dto.ProjectCommentsResponse{Items: items}, nil
}

func (s *Service) CreateProjectComment(projectID uuid.UUID, userID uuid.UUID, req dto.CreateProjectCommentRequest) (*dto.ProjectComment, error) {
	if err := s.requireProjectAccess(projectID, userID); err != nil {
		return nil, err
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return nil, ErrInvalidProjectComment
	}

	metadata, err := JSONMap(req.Metadata)
	if err != nil {
		return nil, err
	}
	comment := models.ProjectComment{
		ProjectID:  projectID,
		AuthorID:   userID,
		Body:       body,
		AnchorText: strings.TrimSpace(req.AnchorText),
		Status:     models.ProjectCommentStatusOpen,
		Metadata:   metadata,
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&comment).Error; err != nil {
			return err
		}
		return RecordProjectActivity(tx, projectID, userID, nil, models.ProjectActivityCommentCreated, map[string]any{
			"comment_id":  comment.ID.String(),
			"anchor_text": comment.AnchorText,
		})
	}); err != nil {
		return nil, err
	}
	return s.getProjectComment(comment.ID)
}

func (s *Service) UpdateProjectComment(projectID uuid.UUID, userID uuid.UUID, commentID uuid.UUID, req dto.UpdateProjectCommentRequest) (*dto.ProjectComment, error) {
	if projectID == uuid.Nil || commentID == uuid.Nil {
		return nil, ErrInvalidProjectComment
	}
	var project models.Project
	if err := s.db.Select("id", "user_id", "workspace_id").First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}
	role, err := accesspolicy.ProjectAccessRoleWithDB(s.db, project, userID)
	if err != nil {
		return nil, err
	}
	if !accesspolicy.CanEditProjectRole(role) {
		return nil, accesspolicy.ErrForbidden
	}
	if strings.TrimSpace(req.Status) != models.ProjectCommentStatusResolved {
		return nil, ErrInvalidProjectComment
	}

	now := time.Now().UTC()
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.ProjectComment{}).
			Where("id = ? AND project_id = ?", commentID, projectID).
			Updates(map[string]any{
				"status":      models.ProjectCommentStatusResolved,
				"resolved_at": &now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return RecordProjectActivity(tx, projectID, userID, nil, models.ProjectActivityCommentResolved, map[string]any{
			"comment_id": commentID.String(),
		})
	}); err != nil {
		return nil, err
	}
	return s.getProjectComment(commentID)
}

func (s *Service) getProjectComment(commentID uuid.UUID) (*dto.ProjectComment, error) {
	var comment models.ProjectComment
	if err := s.db.Preload("Author", selectUserIdentity).First(&comment, "id = ?", commentID).Error; err != nil {
		return nil, err
	}
	item := projectCommentFromModel(comment)
	return &item, nil
}

func projectCommentFromModel(comment models.ProjectComment) dto.ProjectComment {
	return dto.ProjectComment{
		ID:             comment.ID,
		ProjectID:      comment.ProjectID,
		AuthorID:       comment.AuthorID,
		AuthorUsername: comment.Author.Username,
		AuthorEmail:    comment.Author.Email,
		Body:           comment.Body,
		AnchorText:     comment.AnchorText,
		Status:         comment.Status,
		Metadata:       MapFromJSON(comment.Metadata),
		CreatedAt:      comment.CreatedAt,
		ResolvedAt:     comment.ResolvedAt,
	}
}

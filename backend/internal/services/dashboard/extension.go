package dashboard

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"strings"
	"time"
)

const (
	extensionDouyinAdapterKey     = "DYNAMIC_DOUYIN"
	extensionArticleContentKind   = "article"
	extensionPreviewLimit         = 80
	extensionHandoffSchemaVersion = 1
	extensionHandoffType          = "mpp.extension_publish_handoff"
	extensionDouyinInjectURL      = "https://creator.douyin.com/creator-micro/content/upload?default-tab=5"
	extensionHandoffTTL           = 10 * time.Minute
)

func (s *DashboardService) GetExtensionSession(userID uuid.UUID) (*dto.ExtensionSessionResponse, error) {
	var user models.User
	if err := s.db.Select("id", "username").First(&user, "id = ?", userID).Error; err != nil {
		return nil, err
	}

	return &dto.ExtensionSessionResponse{
		Authenticated: true,
		User: dto.ExtensionSessionUser{
			ID:       user.ID,
			Username: user.Username,
		},
	}, nil
}

func (s *DashboardService) ListExtensionPrepublish(userID uuid.UUID) (*dto.ExtensionPrepublishResponse, error) {
	var projects []models.Project
	if err := s.db.
		Joins("JOIN project_platform_publications ppp ON ppp.project_id = projects.id AND ppp.platform = ?", "douyin").
		Where("projects.user_id = ?", userID).
		Preload("Publications", "platform = ?", "douyin").
		Order("projects.updated_at desc").
		Find(&projects).Error; err != nil {
		return nil, err
	}

	items := make([]dto.ExtensionPrepublishItem, 0, len(projects))
	for _, project := range projects {
		platforms := make([]dto.ExtensionPrepublishPlatform, 0, len(project.Publications))
		for _, publication := range project.Publications {
			platforms = append(platforms, extensionPrepublishPlatformFromPublication(publication))
		}
		if len(platforms) == 0 {
			continue
		}
		items = append(items, dto.ExtensionPrepublishItem{
			ProjectID: project.ID,
			Title:     project.Title,
			Status:    project.Status,
			UpdatedAt: project.UpdatedAt,
			Platforms: platforms,
		})
	}

	return &dto.ExtensionPrepublishResponse{Items: items}, nil
}

func extensionPrepublishPlatformFromPublication(publication models.ProjectPlatformPublication) dto.ExtensionPrepublishPlatform {
	return dto.ExtensionPrepublishPlatform{
		PublicationID: publication.ID,
		Platform:      publication.Platform,
		AdapterKey:    extensionDouyinAdapterKey,
		ContentKind:   extensionArticleContentKind,
		Status:        publication.Status,
		Enabled:       publication.Enabled,
		Preview:       extensionPrepublishPreview(publication.AdaptedContent),
	}
}

func extensionPrepublishPreview(raw datatypes.JSON) string {
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}

	for _, key := range []string{"text", "markdown", "html", "summary"} {
		value, ok := payload[key].(string)
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value != "" {
			return truncateRunes(value, extensionPreviewLimit)
		}
	}

	return ""
}

func (s *DashboardService) CreateExtensionHandoff(userID uuid.UUID, req dto.CreateExtensionHandoffRequest, callbackURL string) (*dto.ExtensionPublishHandoff, error) {
	if req.ProjectID == uuid.Nil || len(req.Platforms) == 0 {
		return nil, ErrInvalidProject
	}
	platforms, err := normalizeExtensionHandoffPlatforms(req.Platforms)
	if err != nil {
		return nil, err
	}

	var project models.Project
	if err := s.db.Select("id", "user_id", "title").First(&project, "id = ?", req.ProjectID).Error; err != nil {
		return nil, err
	}
	if project.UserID != userID {
		return nil, ErrForbidden
	}

	executionID := uuid.NewString()
	expiresAt := time.Now().UTC().Add(extensionHandoffTTL)
	handoffPlatforms := make([]dto.ExtensionHandoffPlatform, 0, len(platforms))
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, platform := range platforms {
			var publication models.ProjectPlatformPublication
			if err := tx.Where("project_id = ? AND platform = ?", project.ID, platform).First(&publication).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrPublicationRequiresSync
				}
				return err
			}
			if !publication.Enabled || publication.Status == models.PublicationStatusDisabled {
				return ErrPublicationDisabled
			}
			adaptedContent, err := extensionHandoffAdaptedContent(publication.AdaptedContent)
			if err != nil {
				return err
			}
			callbackToken := uuid.NewString()
			if err := tx.Create(&models.ExtensionCallbackToken{
				ExecutionID: executionID,
				ProjectID:   project.ID,
				UserID:      userID,
				Platform:    platform,
				Token:       callbackToken,
				ExpiresAt:   expiresAt,
			}).Error; err != nil {
				return err
			}
			handoffPlatforms = append(handoffPlatforms, dto.ExtensionHandoffPlatform{
				Platform:       platform,
				AdapterKey:     extensionDouyinAdapterKey,
				InjectURL:      extensionDouyinInjectURL,
				ContentKind:    extensionArticleContentKind,
				AutoPublish:    false,
				RequiresReview: true,
				AdaptedContent: adaptedContent,
				Assets:         []dto.ExtensionHandoffAsset{},
				Callback: dto.ExtensionHandoffCallback{
					URL:   callbackURL,
					Token: callbackToken,
				},
			})
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return &dto.ExtensionPublishHandoff{
		SchemaVersion: extensionHandoffSchemaVersion,
		Type:          extensionHandoffType,
		ExecutionID:   executionID,
		ExpiresAt:     expiresAt,
		Project: dto.ExtensionHandoffProject{
			ID:    project.ID,
			Title: project.Title,
		},
		Platforms: handoffPlatforms,
	}, nil
}

func (s *DashboardService) RecordExtensionEvent(req dto.ExtensionEventCallbackRequest) (*dto.ExtensionEventCallbackResponse, error) {
	tokenValue := strings.TrimSpace(req.Token)
	eventID := strings.TrimSpace(req.EventID)
	platform := strings.TrimSpace(req.Platform)
	status := strings.TrimSpace(req.Status)
	if tokenValue == "" || eventID == "" || platform == "" || status == "" {
		return nil, ErrExtensionCallbackTokenInvalid
	}

	var token models.ExtensionCallbackToken
	if err := s.db.First(&token, "token = ?", tokenValue).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrExtensionCallbackTokenInvalid
		}
		return nil, err
	}
	if time.Now().UTC().After(token.ExpiresAt) {
		return nil, ErrExtensionCallbackTokenExpired
	}
	if token.Platform != platform {
		return nil, ErrExtensionCallbackTokenInvalid
	}

	var existing models.ExtensionExecutionEvent
	if err := s.db.First(&existing, "event_id = ?", eventID).Error; err == nil {
		return &dto.ExtensionEventCallbackResponse{Accepted: true, Duplicate: true}, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	metadata := datatypes.JSON([]byte(`{}`))
	if req.Metadata != nil {
		payload, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, err
		}
		metadata = datatypes.JSON(payload)
	}

	if err := s.db.Create(&models.ExtensionExecutionEvent{
		CallbackTokenID: token.ID,
		ExecutionID:     token.ExecutionID,
		ProjectID:       token.ProjectID,
		UserID:          token.UserID,
		EventID:         eventID,
		Platform:        platform,
		Status:          status,
		Message:         strings.TrimSpace(req.Message),
		RemoteID:        strings.TrimSpace(req.RemoteID),
		PublishURL:      strings.TrimSpace(req.PublishURL),
		ErrorMessage:    strings.TrimSpace(req.ErrorMessage),
		Metadata:        metadata,
	}).Error; err != nil {
		return nil, err
	}

	return &dto.ExtensionEventCallbackResponse{Accepted: true, Duplicate: false}, nil
}

func normalizeExtensionHandoffPlatforms(input []string) ([]string, error) {
	seen := map[string]struct{}{}
	platforms := make([]string, 0, len(input))
	for _, raw := range input {
		platform := strings.TrimSpace(raw)
		if platform == "" {
			continue
		}
		if platform != "douyin" {
			return nil, ErrInvalidProject
		}
		if _, ok := seen[platform]; ok {
			continue
		}
		seen[platform] = struct{}{}
		platforms = append(platforms, platform)
	}
	if len(platforms) == 0 {
		return nil, ErrInvalidProject
	}
	return platforms, nil
}

func extensionHandoffAdaptedContent(raw datatypes.JSON) (map[string]interface{}, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, ErrPublicationRequiresSync
	}
	text, ok := payload["text"].(string)
	text = strings.TrimSpace(text)
	if !ok || text == "" {
		return nil, ErrPublicationRequiresSync
	}
	return map[string]interface{}{
		"schema_version": extensionHandoffSchemaVersion,
		"format":         "text",
		"text":           text,
	}, nil
}

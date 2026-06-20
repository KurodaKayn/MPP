package extension

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/textutil"
	platformcapabilities "github.com/kurodakayn/mpp-backend/internal/platformcapabilities"
	publishsvc "github.com/kurodakayn/mpp-backend/internal/services/publish"
)

const (
	extensionPreviewLimit         = 80
	extensionHandoffSchemaVersion = 1
	extensionHandoffType          = "mpp.extension_publish_handoff"
	extensionHandoffTTL           = 10 * time.Minute
)

var extensionPlatformKeys = platformcapabilities.ExtensionHandoffPlatformKeys

func (s *Service) GetExtensionSession(userID uuid.UUID) (*dto.ExtensionSessionResponse, error) {
	var user models.User
	if err := s.strongReadDB().Select("id", "username").First(&user, "id = ?", userID).Error; err != nil {
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

func (s *Service) ListExtensionPrepublish(userID uuid.UUID) (*dto.ExtensionPrepublishResponse, error) {
	var projects []models.Project
	if err := s.eventualReadDB().
		Distinct("projects.*").
		Joins("JOIN project_platform_publications ppp ON ppp.project_id = projects.id AND ppp.platform IN ?", extensionPlatformKeys).
		Where("projects.user_id = ?", userID).
		Preload("Publications", "platform IN ?", extensionPlatformKeys).
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
	config, _ := platformcapabilities.ExtensionHandoffConfigFor(publication.Platform)

	return dto.ExtensionPrepublishPlatform{
		PublicationID: publication.ID,
		Platform:      publication.Platform,
		AdapterKey:    config.AdapterKey,
		ContentKind:   config.ContentKind,
		Status:        publication.Status,
		Enabled:       publication.Enabled,
		Preview:       extensionPrepublishPreview(publication.AdaptedContent),
	}
}

func extensionPrepublishPreview(raw datatypes.JSON) string {
	var payload map[string]any
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
			return textutil.TruncateRunes(value, extensionPreviewLimit)
		}
	}

	return ""
}

func (s *Service) CreateExtensionHandoff(userID uuid.UUID, req dto.CreateExtensionHandoffRequest, callbackURL string) (*dto.ExtensionPublishHandoff, error) {
	if req.ProjectID == uuid.Nil || len(req.Platforms) == 0 {
		return nil, ErrInvalidProject
	}
	platforms, err := normalizeExtensionHandoffPlatforms(req.Platforms)
	if err != nil {
		return nil, err
	}

	var project models.Project
	if err := s.strongReadDB().Select("id", "user_id", "title").First(&project, "id = ?", req.ProjectID).Error; err != nil {
		return nil, err
	}
	if project.UserID != userID {
		return nil, ErrForbidden
	}

	executionID := uuid.NewString()
	expiresAt := time.Now().UTC().Add(extensionHandoffTTL)
	handoffPlatforms := make([]dto.ExtensionHandoffPlatform, 0, len(platforms))
	if err := s.writerDB().Transaction(func(tx *gorm.DB) error {
		for _, platform := range platforms {
			config, _ := platformcapabilities.ExtensionHandoffConfigFor(platform)
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
				AdapterKey:     config.AdapterKey,
				InjectURL:      config.InjectURL,
				ContentKind:    config.ContentKind,
				AutoPublish:    config.AutoPublish,
				RequiresReview: config.RequiresReview,
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

func (s *Service) RecordExtensionEvent(req dto.ExtensionEventCallbackRequest) (*dto.ExtensionEventCallbackResponse, error) {
	tokenValue := strings.TrimSpace(req.Token)
	eventID := strings.TrimSpace(req.EventID)
	platform := strings.TrimSpace(req.Platform)
	status := strings.TrimSpace(req.Status)
	if tokenValue == "" || eventID == "" || platform == "" || status == "" {
		return nil, ErrExtensionCallbackTokenInvalid
	}

	var token models.ExtensionCallbackToken
	if err := s.writerDB().First(&token, "token = ?", tokenValue).Error; err != nil {
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
	if err := s.writerDB().First(&existing, "event_id = ?", eventID).Error; err == nil {
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

	event := models.ExtensionExecutionEvent{
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
	}
	if err := s.writerDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		return applyExtensionPublicationEvent(tx, token, event)
	}); err != nil {
		return nil, err
	}

	return &dto.ExtensionEventCallbackResponse{Accepted: true, Duplicate: false}, nil
}

func applyExtensionPublicationEvent(tx *gorm.DB, token models.ExtensionCallbackToken, event models.ExtensionExecutionEvent) error {
	updates := extensionPublicationUpdatesForEvent(event)
	if len(updates) == 0 {
		return nil
	}

	return tx.Model(&models.ProjectPlatformPublication{}).
		Where("project_id = ? AND platform = ?", token.ProjectID, token.Platform).
		Updates(updates).Error
}

func extensionPublicationUpdatesForEvent(event models.ExtensionExecutionEvent) map[string]any {
	switch event.Status {
	case "user_review":
		updates := map[string]any{
			"status":        models.PublicationStatusAdapted,
			"draft_status":  models.PublicationDraftStatusReady,
			"review_status": models.PublicationReviewStatusReviewing,
			"error_message": "",
		}
		addOptionalExtensionPublicationRefs(updates, event)
		return updates
	case "failed":
		message := extensionEventErrorMessage(event)
		updates := map[string]any{
			"status":        models.PublicationStatusFailed,
			"error_message": publishsvc.SanitizeUserFacingErrorMessage(message),
			"retry_count":   gorm.Expr("retry_count + ?", 1),
		}
		addOptionalExtensionPublicationRefs(updates, event)
		return updates
	default:
		return nil
	}
}

func addOptionalExtensionPublicationRefs(updates map[string]any, event models.ExtensionExecutionEvent) {
	if event.RemoteID != "" {
		updates["remote_id"] = event.RemoteID
	}
	if event.PublishURL != "" {
		updates["publish_url"] = event.PublishURL
	}
}

func extensionEventErrorMessage(event models.ExtensionExecutionEvent) string {
	if event.ErrorMessage != "" {
		return event.ErrorMessage
	}
	if event.Message != "" {
		return event.Message
	}
	return "extension adapter failed"
}

func normalizeExtensionHandoffPlatforms(input []string) ([]string, error) {
	seen := map[string]struct{}{}
	platforms := make([]string, 0, len(input))
	for _, raw := range input {
		platform := strings.TrimSpace(raw)
		if platform == "" {
			continue
		}
		if _, ok := platformcapabilities.ExtensionHandoffConfigFor(platform); !ok {
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

func extensionHandoffAdaptedContent(raw datatypes.JSON) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, ErrPublicationRequiresSync
	}
	text, ok := payload["text"].(string)
	text = strings.TrimSpace(text)
	if !ok || text == "" {
		return nil, ErrPublicationRequiresSync
	}
	return map[string]any{
		"schema_version": extensionHandoffSchemaVersion,
		"format":         "text",
		"text":           text,
	}, nil
}

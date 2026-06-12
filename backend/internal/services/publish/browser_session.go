package publish

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/media"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	publishercontent "github.com/kurodakayn/mpp-backend/internal/publisher/content"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	platformaccount "github.com/kurodakayn/mpp-backend/internal/services/platform_account"
)

func (s *Service) StartDouyinPublishSession(ctx context.Context, projectID uuid.UUID, userID uuid.UUID) (*dto.StartBrowserSessionResponse, error) {
	if s.browserSessionService == nil {
		return nil, browsersession.ErrPlatformNotSupported
	}
	project, err := s.projectForPublish(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	var pub models.ProjectPlatformPublication
	if err := s.db.WithContext(ctx).Where("project_id = ? AND platform = ?", projectID, "douyin").First(&pub).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPublicationRequiresSync
		}
		return nil, err
	}
	if !pub.Enabled || pub.Status == models.PublicationStatusDisabled {
		return nil, ErrPublicationDisabled
	}
	if len(pub.AdaptedContent) == 0 || string(pub.AdaptedContent) == "{}" {
		return nil, ErrPublicationRequiresSync
	}
	if s.browserWorkerClient == nil {
		return nil, browsersession.ErrPlatformNotSupported
	}
	account, err := s.accounts.ResolvePublicationAccount(userID, &pub)
	if err != nil {
		if errors.Is(err, platformaccount.ErrPlatformAccountForbidden) {
			return nil, ErrForbidden
		}
		return nil, err
	}
	pub, err = s.preparePublicationMediaRefs(ctx, project, pub)
	if err != nil {
		return nil, err
	}
	draft, err := buildDouyinWorkerDraft(ctx, project, pub)
	if err != nil {
		return nil, err
	}

	workspaceID := uuid.Nil
	if project.WorkspaceID != nil {
		workspaceID = *project.WorkspaceID
	}
	resp, err := s.browserSessionService.StartPreauthorizedSessionForWorkspace(ctx, userID, "", workspaceID, account.ID, "douyin")
	if err != nil {
		if !errors.Is(err, browsersession.ErrActiveSessionExists) {
			return nil, err
		}
		if cleanupErr := s.cancelActiveDouyinBrowserSessions(ctx, userID); cleanupErr != nil {
			return nil, cleanupErr
		}
		resp, err = s.browserSessionService.StartPreauthorizedSessionForWorkspace(ctx, userID, "", workspaceID, account.ID, "douyin")
		if err != nil {
			return nil, err
		}
	}

	var browserSession models.RemoteBrowserSession
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", resp.SessionID, userID).First(&browserSession).Error; err != nil {
		return nil, err
	}

	if err := s.browserWorkerClient.StartDouyinPublish(ctx, browserSession.WorkerSessionRef, draft); err != nil {
		return nil, fmt.Errorf("failed to start douyin publish script: %w", err)
	}

	return resp, nil
}

func (s *Service) cancelActiveDouyinBrowserSessions(ctx context.Context, userID uuid.UUID) error {
	var sessions []models.RemoteBrowserSession
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND platform = ? AND status IN ?", userID, "douyin", []string{
			models.BrowserSessionStatusPending,
			models.BrowserSessionStatusReady,
			models.BrowserSessionStatusLoginDetected,
			models.BrowserSessionStatusCapturing,
		}).
		Find(&sessions).Error; err != nil {
		return err
	}

	for _, session := range sessions {
		if err := s.browserSessionService.CancelSession(ctx, userID, session.ID); err != nil && !errors.Is(err, browsersession.ErrSessionNotFound) {
			return err
		}
	}
	return nil
}

func buildDouyinWorkerDraft(ctx context.Context, project models.Project, pub models.ProjectPlatformPublication) (publisher.StartDouyinPublishRequest, error) {
	title := publishercontent.ExtractPublicationTitle(pub.Config)
	if title == "" {
		title = strings.TrimSpace(project.Title)
	}
	if title == "" {
		title = "抖音图文"
	}

	content := extractDouyinWorkerText(pub.AdaptedContent)
	if content == "" {
		return publisher.StartDouyinPublishRequest{}, fmt.Errorf("douyin text content is empty")
	}

	imageData, imageName, err := douyinWorkerCoverImage(ctx, pub.Config)
	if err != nil {
		return publisher.StartDouyinPublishRequest{}, err
	}

	return publisher.StartDouyinPublishRequest{
		Title:            title,
		Content:          content,
		CoverImageBase64: base64.StdEncoding.EncodeToString(imageData),
		CoverImageName:   imageName,
	}, nil
}

func extractDouyinWorkerText(raw []byte) string {
	var structured publisher.AdaptedContent
	if err := json.Unmarshal(raw, &structured); err == nil {
		if structured.Text != nil {
			if text := strings.TrimSpace(*structured.Text); text != "" {
				return text
			}
		}
		if structured.Summary != nil {
			if summary := strings.TrimSpace(*structured.Summary); summary != "" {
				return summary
			}
		}
	}

	var plain string
	if err := json.Unmarshal(raw, &plain); err == nil {
		return strings.TrimSpace(plain)
	}

	return strings.TrimSpace(string(raw))
}

func douyinWorkerCoverImage(ctx context.Context, rawConfig []byte) ([]byte, string, error) {
	var config struct {
		CoverImageURL string `json:"cover_image_url"`
	}
	_ = json.Unmarshal(rawConfig, &config)

	if source := strings.TrimSpace(config.CoverImageURL); source != "" {
		objectRef, err := media.DownloadAndProcess(source)
		if err != nil {
			return nil, "", fmt.Errorf("failed to prepare douyin cover image: %w", err)
		}
		data, err := media.ReadProcessedObject(ctx, objectRef)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read douyin cover image: %w", err)
		}
		return data, filepath.Base(source), nil
	}

	path, err := bundledDouyinWorkerImagePath()
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(path) //nolint:gosec // Path is selected from fixed bundled asset candidates.
	if err != nil {
		return nil, "", fmt.Errorf("failed to read douyin cover image: %w", err)
	}
	return data, filepath.Base(path), nil
}

func bundledDouyinWorkerImagePath() (string, error) {
	name := "132461906_p0_master1200.jpg"
	candidates := []string{
		filepath.Join("backend", "Assets", name),
		filepath.Join("Assets", name),
		filepath.Join("..", "..", "Assets", name),
		filepath.Join("..", "..", "..", "Assets", name),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("douyin image publish requires a cover image")
}

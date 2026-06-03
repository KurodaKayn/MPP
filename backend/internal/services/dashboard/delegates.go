package dashboard

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
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/media"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	publishercontent "github.com/kurodakayn/mpp-backend/internal/publisher/content"
	browsersession "github.com/kurodakayn/mpp-backend/internal/services/browser_session"
	"gorm.io/gorm"
)

func (s *DashboardService) BatchPublishProject(projectID uuid.UUID, platforms []string, scopeUserID *uuid.UUID) (map[string]map[string]interface{}, error) {
	return s.publisher.BatchPublishProject(projectID, platforms, scopeUserID)
}

func (s *DashboardService) PublishProject(projectID uuid.UUID, platform string, scopeUserID *uuid.UUID, browserSessionID uuid.UUID) (map[string]interface{}, error) {
	return s.publisher.PublishProject(projectID, platform, scopeUserID, browserSessionID)
}

func (s *DashboardService) CreateXPostIntent(projectID uuid.UUID, scopeUserID *uuid.UUID) (map[string]interface{}, error) {
	return s.publisher.CreateXPostIntent(projectID, scopeUserID)
}

func (s *DashboardService) EnqueuePublishProject(ctx context.Context, projectID uuid.UUID, platform string, scopeUserID *uuid.UUID) (map[string]interface{}, error) {
	return s.publisher.EnqueuePublishProject(ctx, projectID, platform, scopeUserID)
}

func (s *DashboardService) BatchEnqueuePublishProject(ctx context.Context, projectID uuid.UUID, platforms []string, scopeUserID *uuid.UUID) (map[string]map[string]interface{}, error) {
	return s.publisher.BatchEnqueuePublishProject(ctx, projectID, platforms, scopeUserID)
}

func (s *DashboardService) StartPublishWorker(ctx context.Context) {
	s.publisher.StartPublishWorker(ctx)
}

func (s *DashboardService) StartDouyinPublishSession(ctx context.Context, projectID uuid.UUID, userID uuid.UUID) (*dto.StartBrowserSessionResponse, error) {
	if s.browserSessionService == nil {
		return nil, browsersession.ErrPlatformNotSupported
	}
	var project models.Project
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", projectID, userID).First(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrForbidden
		}
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
	draft, err := buildDouyinWorkerDraft(project, pub)
	if err != nil {
		return nil, err
	}

	resp, err := s.browserSessionService.StartSession(ctx, userID, "douyin")
	if err != nil {
		if !errors.Is(err, browsersession.ErrActiveSessionExists) {
			return nil, err
		}
		if cleanupErr := s.cancelActiveDouyinBrowserSessions(ctx, userID); cleanupErr != nil {
			return nil, cleanupErr
		}
		resp, err = s.browserSessionService.StartSession(ctx, userID, "douyin")
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

func (s *DashboardService) cancelActiveDouyinBrowserSessions(ctx context.Context, userID uuid.UUID) error {
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

func buildDouyinWorkerDraft(project models.Project, pub models.ProjectPlatformPublication) (publisher.StartDouyinPublishRequest, error) {
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

	imageData, imageName, err := douyinWorkerCoverImage(pub.Config)
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

func douyinWorkerCoverImage(rawConfig []byte) ([]byte, string, error) {
	var config struct {
		CoverImageURL string `json:"cover_image_url"`
	}
	_ = json.Unmarshal(rawConfig, &config)

	if source := strings.TrimSpace(config.CoverImageURL); source != "" {
		data, err := media.DownloadAndProcess(source)
		if err != nil {
			return nil, "", fmt.Errorf("failed to prepare douyin cover image: %w", err)
		}
		return data, filepath.Base(source), nil
	}

	path, err := bundledDouyinWorkerImagePath()
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(path)
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

func (s *DashboardService) GetWechatAccount(userID uuid.UUID) (*dto.WechatAccountResponse, error) {
	return s.accounts.GetWechatAccount(userID)
}

func (s *DashboardService) UpsertWechatAccount(userID uuid.UUID, req dto.UpsertWechatAccountRequest) (*dto.WechatAccountResponse, error) {
	return s.accounts.UpsertWechatAccount(userID, req)
}

func (s *DashboardService) TestWechatAccount(userID uuid.UUID, req dto.TestWechatAccountRequest) (*dto.WechatConnectionTestResponse, error) {
	return s.accounts.TestWechatAccount(userID, req)
}

func (s *DashboardService) GetDouyinAccount(userID uuid.UUID) (*dto.DouyinAccountResponse, error) {
	return s.accounts.GetDouyinAccount(userID)
}

func (s *DashboardService) GetZhihuAccount(userID uuid.UUID) (*dto.ZhihuAccountResponse, error) {
	return s.accounts.GetZhihuAccount(userID)
}

func (s *DashboardService) GetXAccount(userID uuid.UUID) (*dto.XAccountResponse, error) {
	return s.accounts.GetXAccount(userID)
}

func (s *DashboardService) UpsertXAccount(userID uuid.UUID, req dto.UpsertXAccountRequest) (*dto.XAccountResponse, error) {
	return s.accounts.UpsertXAccount(userID, req)
}

func (s *DashboardService) TestXAccount(userID uuid.UUID, req dto.TestXAccountRequest) (*dto.XConnectionTestResponse, error) {
	return s.accounts.TestXAccount(userID, req)
}

func (s *DashboardService) StartXOAuth2(userID uuid.UUID, redirectURI string) (string, error) {
	return s.accounts.StartXOAuth2(userID, redirectURI)
}

func (s *DashboardService) CompleteXOAuth2(ctx context.Context, state, code string) (*dto.XAccountResponse, error) {
	return s.accounts.CompleteXOAuth2(ctx, state, code)
}

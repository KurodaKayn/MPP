package publisher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kurodakayn/sevenoxcloud-backend/internal/models"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/pkg/html"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/pkg/media"
	"github.com/kurodakayn/sevenoxcloud-backend/internal/pkg/wechat"
)

type WechatPublisher struct{}

type WechatConfig struct {
	AppID         string `json:"app_id"`
	AppSecret     string `json:"app_secret"`
	Title         string `json:"title"`
	Author        string `json:"author"`
	Digest        string `json:"digest"`
	CoverImageURL string `json:"cover_image_url"`
}

func (w *WechatPublisher) ValidateConfig(config []byte) error {
	var cfg WechatConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return err
	}
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return fmt.Errorf("app_id and app_secret are required")
	}
	return nil
}

func (w *WechatPublisher) AdaptContent(project *models.Project) ([]byte, error) {
	// For WeChat, the "adapted content" will be the HTML processed with WeChat image URLs.
	// However, since image processing requires the Access Token (which is tied to a specific user's config),
	// it's better to do the heavy lifting in the Publish phase.
	// For now, we return a simple JSON indicating it's ready.
	return []byte(`{"status": "ready_to_process"}`), nil
}

func (w *WechatPublisher) Publish(ctx context.Context, pub *models.ProjectPlatformPublication) (string, string, error) {
	var cfg WechatConfig
	if err := json.Unmarshal(pub.Config, &cfg); err != nil {
		return "", "", fmt.Errorf("failed to parse wechat config: %w", err)
	}

	client := wechat.NewClient(cfg.AppID, cfg.AppSecret)

	// 1. Process HTML images (Download -> Compress -> Upload to WeChat -> Replace URL)
	processedHTML, err := html.ProcessHTMLImages(
		string(pub.AdaptedContent), // Or from project.SourceContent if AdaptedContent is empty
		media.DownloadAndProcess,
		func(imgData []byte) (string, error) {
			res, err := client.UploadImage(imgData, "content_image.jpg")
			if err != nil {
				return "", err
			}
			return res.URL, nil
		},
	)
	if err != nil {
		// Fallback to original content if processing fails, or return error
		processedHTML = string(pub.AdaptedContent)
	}

	// 2. Upload Cover Image for thumb_media_id
	var thumbMediaID string
	if cfg.CoverImageURL != "" {
		coverData, err := media.DownloadAndProcess(cfg.CoverImageURL)
		if err == nil {
			res, err := client.UploadImage(coverData, "cover.jpg")
			if err == nil {
				thumbMediaID = res.MediaID
			}
		}
	}

	// 3. Create Draft
	articles := []wechat.Article{
		{
			Title:              cfg.Title,
			ThumbMediaID:       thumbMediaID,
			Author:             cfg.Author,
			Digest:             cfg.Digest,
			Content:            processedHTML,
			NeedOpenComment:    1,
			OnlyFansCanComment: 0,
		},
	}
	draftMediaID, err := client.CreateDraft(articles)
	if err != nil {
		return "", "", fmt.Errorf("failed to create draft: %w", err)
	}

	// 4. Submit for Publication
	publishID, errCode, err := client.Publish(draftMediaID)
	if err != nil {
		return draftMediaID, "", fmt.Errorf("failed to submit for publish: %w", err)
	}

	// Handle special error code 48001 (Unauthorized API publishing)
	if errCode == 48001 {
		warningMsg := "Draft created successfully (MediaID: " + draftMediaID + "), but your account requires manual publication via WeChat Dashboard (Error 48001)."
		return draftMediaID, "", errors.New(warningMsg)
	}

	publishURL := fmt.Sprintf("https://mp.weixin.qq.com/s?publish_id=%s", publishID)
	return draftMediaID, publishURL, nil
}

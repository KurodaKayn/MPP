package wechat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kurodakayn/mpp-backend/internal/models"
	htmlutil "github.com/kurodakayn/mpp-backend/internal/pkg/html"
	"github.com/kurodakayn/mpp-backend/internal/pkg/media"
	pkgwechat "github.com/kurodakayn/mpp-backend/internal/pkg/wechat"
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

const defaultCoverImagePath = "Assets/132461906_p0_master1200.jpg"

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

func (w *WechatPublisher) Publish(ctx context.Context, pub *models.ProjectPlatformPublication, _ *models.PlatformAccount) (string, string, error) {
	var cfg WechatConfig
	if err := json.Unmarshal(pub.Config, &cfg); err != nil {
		return "", "", fmt.Errorf("failed to parse wechat config: %w", err)
	}

	client := pkgwechat.NewClient(cfg.AppID, cfg.AppSecret)
	sourceHTML := extractWechatHTML(pub.AdaptedContent)

	// 1. Process HTML images through content-pipeline-service, upload to WeChat, and replace URLs.
	processedHTML, err := htmlutil.ProcessHTMLImages(
		sourceHTML,
		media.DownloadAndProcess,
		func(objectRef string) (string, error) {
			imgData, err := media.ReadProcessedObject(ctx, objectRef)
			if err != nil {
				return "", err
			}
			res, err := client.UploadImage(imgData, "content_image.jpg")
			if err != nil {
				return "", err
			}
			return res.URL, nil
		},
	)
	if err != nil {
		processedHTML = string(pub.AdaptedContent)
	}

	// 2. Upload cover image for thumb_media_id.
	coverData, err := loadCoverImage(ctx, cfg.CoverImageURL)
	if err != nil {
		return "", "", err
	}
	res, err := client.UploadThumb(coverData, "cover.jpg")
	if err != nil {
		return "", "", fmt.Errorf("failed to upload wechat cover image: %w", err)
	}
	thumbMediaID := res.MediaID
	if strings.TrimSpace(thumbMediaID) == "" {
		return "", "", fmt.Errorf("failed to upload wechat cover image: empty thumb media id")
	}

	// 3. Create Draft
	articles := []pkgwechat.Article{
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
		return draftMediaID, "", fmt.Errorf("缺乏企业或个人认证统一发布资格，请自行前往草稿箱发布（文章已撰写完成）")
	}

	publishURL := fmt.Sprintf("https://mp.weixin.qq.com/s?publish_id=%s", publishID)
	return draftMediaID, publishURL, nil
}

func loadCoverImage(ctx context.Context, coverImageURL string) ([]byte, error) {
	if strings.TrimSpace(coverImageURL) != "" {
		coverObjectRef, err := media.DownloadAndProcess(coverImageURL)
		if err != nil {
			return nil, fmt.Errorf("failed to download wechat cover image: %w", err)
		}
		coverData, err := media.ReadProcessedObject(ctx, coverObjectRef)
		if err != nil {
			return nil, fmt.Errorf("failed to read wechat cover image: %w", err)
		}
		return coverData, nil
	}

	coverData, err := os.ReadFile(defaultCoverImagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read default wechat cover image: %w", err)
	}
	return coverData, nil
}

func extractWechatHTML(adaptedContent []byte) string {
	var structured struct {
		Content string `json:"content"`
		HTML    string `json:"html"`
	}
	if err := json.Unmarshal(adaptedContent, &structured); err == nil {
		if structured.HTML != "" {
			return structured.HTML
		}
		if structured.Content != "" {
			return structured.Content
		}
	}

	var plain string
	if err := json.Unmarshal(adaptedContent, &plain); err == nil {
		return plain
	}

	return string(adaptedContent)
}

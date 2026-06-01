package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/media"
)

type DouyinPublisher struct{}

func (d *DouyinPublisher) ValidateConfig(config []byte) error {
	// Douyin specific configuration validation (e.g. hashtags, publish time)
	return nil
}

func (d *DouyinPublisher) AdaptContent(project *models.Project) ([]byte, error) {
	text := htmlToText(project.SourceContent)
	if text == "" {
		text = strings.TrimSpace(project.SourceContent)
	}
	content := systemAdaptedContent(project, "text", "douyin-text-adapter", text)
	content.Text = text
	return json.Marshal(content)
}

func (d *DouyinPublisher) Publish(ctx context.Context, pub *models.ProjectPlatformPublication, account *models.PlatformAccount) (string, string, error) {
	if account == nil {
		return "", "", fmt.Errorf("douyin headless publishing requires an account with cookies")
	}

	title := extractPublicationTitle(pub.Config)
	if title == "" {
		title = "抖音图文"
	}
	content := extractDouyinText(pub.AdaptedContent)
	if content == "" {
		return "", "", fmt.Errorf("douyin text content is empty")
	}
	localImagePath, cleanupImage, err := douyinUploadImagePath(pub.Config)
	if err != nil {
		return "", "", err
	}
	defer cleanupImage()

	// Setup browser with account cookies
	browserCtx, cancel := SetupBrowser(ctx, "", account.Cookies)
	defer cancel()

	publishCtx, cancelPublish := context.WithTimeout(browserCtx, 180*time.Second)
	defer cancelPublish()

	var publishURL string

	err = chromedp.Run(publishCtx,
		// 1. 进入核心上传页
		chromedp.Navigate("https://creator.douyin.com/creator-micro/content/upload?default-tab=3"),
		chromedp.Sleep(5*time.Second),

		// 2. 根据用户提供的 HTML 源码，精准点击“上传图文”按钮
		chromedp.ActionFunc(func(ctx context.Context) error {
			fmt.Println("Douyin: Attempting to click upload button...")
			script := `
				(function() {
					const btn = document.querySelector('.container-drag-btn-k6XmB4') || 
					            document.querySelector('button.semi-button-primary');
					if (btn) {
						btn.click();
						return "Target 'Upload Image Post' button clicked via class";
					}
					const container = document.querySelector('.container-drag-VAfIfu');
					if (container) {
						container.click();
						return "Drag container clicked as fallback";
					}
					return "Upload button NOT found";
				})()
			`
			var res string
			if err := chromedp.Evaluate(script, &res).Do(ctx); err != nil {
				return err
			}
			fmt.Printf("Douyin Action: %s\n", res)
			return nil
		}),
		chromedp.Sleep(2*time.Second),

		// 3. 注入本地图片
		chromedp.ActionFunc(func(ctx context.Context) error {
			fmt.Println("Douyin: Waiting for file input...")
			return nil
		}),
		chromedp.WaitReady(`input[type="file"]`, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			fmt.Printf("Douyin: Uploading file: %s\n", localImagePath)
			return chromedp.SetUploadFiles(`input[type="file"]`, []string{localImagePath}, chromedp.ByQuery).Do(ctx)
		}),
		chromedp.Sleep(10*time.Second), // 图片上传解析时间

		// 4. 开始填写文字：标题
		chromedp.ActionFunc(func(ctx context.Context) error {
			fmt.Println("Douyin: Waiting for title input...")
			return nil
		}),
		chromedp.WaitVisible(`input[placeholder*="标题"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[placeholder*="标题"]`, title),

		// 5. 填写文字：描述正文 (模拟 Ctrl+V)
		chromedp.ActionFunc(func(ctx context.Context) error {
			script := `
				(function() {
					const el = document.querySelector('.public-DraftEditor-content') || 
					           document.querySelector('div[contenteditable="true"]');
					if (el) {
						el.focus();
						return "Description focused";
					}
					return "Description NOT found";
				})()
			`
			return chromedp.Evaluate(script, nil).Do(ctx)
		}),
		PasteContent(`div[contenteditable="true"]`, content, false),
		chromedp.Sleep(3*time.Second),

		// 6. 暂存离开
		chromedp.ActionFunc(func(ctx context.Context) error {
			script := `
				(function() {
					const buttons = Array.from(document.querySelectorAll('button'));
					const draftBtn = buttons.find(b => b.textContent.includes('暂存离开'));
					if (draftBtn) {
						draftBtn.click();
						return "Draft button clicked";
					}
					return "Draft button NOT found";
				})()
			`
			var res string
			err := chromedp.Evaluate(script, &res).Do(ctx)
			fmt.Printf("Douyin Action: %s\n", res)
			return err
		}),

		chromedp.Sleep(10*time.Second),
		chromedp.Location(&publishURL),
	)

	if err != nil {
		return "", "", fmt.Errorf("douyin headless publish failed: %w", err)
	}

	return "dy_headless", publishURL, nil
}

func extractDouyinText(raw []byte) string {
	var structured AdaptedContent
	if err := json.Unmarshal(raw, &structured); err == nil {
		if text := strings.TrimSpace(structured.Text); text != "" {
			return text
		}
		if summary := strings.TrimSpace(structured.Summary); summary != "" {
			return summary
		}
	}

	var plain string
	if err := json.Unmarshal(raw, &plain); err == nil {
		return strings.TrimSpace(plain)
	}

	return strings.TrimSpace(string(raw))
}

func douyinUploadImagePath(rawConfig []byte) (string, func(), error) {
	var config struct {
		CoverImageURL string `json:"cover_image_url"`
	}
	_ = json.Unmarshal(rawConfig, &config)

	if source := strings.TrimSpace(config.CoverImageURL); source != "" {
		data, err := media.DownloadAndProcess(source)
		if err != nil {
			return "", nil, fmt.Errorf("failed to prepare douyin cover image: %w", err)
		}
		file, err := os.CreateTemp("", "mpp-douyin-cover-*")
		if err != nil {
			return "", nil, fmt.Errorf("failed to create douyin cover image: %w", err)
		}
		if _, err := file.Write(data); err != nil {
			_ = file.Close()
			_ = os.Remove(file.Name())
			return "", nil, fmt.Errorf("failed to write douyin cover image: %w", err)
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(file.Name())
			return "", nil, fmt.Errorf("failed to close douyin cover image: %w", err)
		}
		return file.Name(), func() { _ = os.Remove(file.Name()) }, nil
	}

	path, err := bundledDouyinImagePath()
	if err != nil {
		return "", nil, err
	}
	return path, func() {}, nil
}

func bundledDouyinImagePath() (string, error) {
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

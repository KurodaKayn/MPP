package douyin

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
	"github.com/kurodakayn/mpp-backend/internal/publisher/browser"
	"github.com/kurodakayn/mpp-backend/internal/publisher/content"
	"github.com/kurodakayn/mpp-backend/internal/publisher/core"
)

type DouyinPublisher struct{}

func (d *DouyinPublisher) ValidateConfig(config []byte) error {
	// Douyin specific configuration validation (e.g. hashtags, publish time)
	return nil
}

func (d *DouyinPublisher) AdaptContent(project *models.Project) ([]byte, error) {
	text := content.HTMLToText(project.SourceContent)
	if text == "" {
		text = strings.TrimSpace(project.SourceContent)
	}
	adapted := core.SystemAdaptedContent(project, "text", "douyin-text-adapter", text)
	adapted.Text = core.String(text)
	return json.Marshal(adapted)
}

func (d *DouyinPublisher) Publish(ctx context.Context, pub *models.ProjectPlatformPublication, account *models.PlatformAccount) (string, string, error) {
	if account == nil {
		return "", "", fmt.Errorf("douyin headless publishing requires an account with cookies")
	}

	title := content.ExtractPublicationTitle(pub.Config)
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
	browserCtx, cancel := browser.SetupBrowser(ctx, "", account.Cookies)
	defer cancel()

	publishCtx, cancelPublish := context.WithTimeout(browserCtx, 300*time.Second)
	defer cancelPublish()

	var publishURL string

	err = chromedp.Run(publishCtx,
		// 1. 进入核心上传页
		chromedp.Navigate("https://creator.douyin.com/creator-micro/content/upload?default-tab=3"),

		// 2. 等待用户完成可能出现的扫码登录，然后点击“上传图文”按钮
		chromedp.ActionFunc(func(ctx context.Context) error {
			fmt.Println("Douyin: Attempting to click upload button...")
			waitCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
			defer cancel()
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				script := `
					(function() {
						const btn = document.querySelector('.container-drag-btn-k6XmB4') || 
									document.querySelector('button.semi-button-primary');
						if (btn) {
							btn.click();
							return "clicked";
						}
						const container = document.querySelector('.container-drag-VAfIfu');
						if (container) {
							container.click();
							return "clicked";
						}
						return "waiting";
					})()
				`
				var res string
				if err := chromedp.Evaluate(script, &res).Do(waitCtx); err != nil {
					return err
				}
				fmt.Printf("Douyin Action: %s\n", res)
				if res == "clicked" {
					return nil
				}
				select {
				case <-waitCtx.Done():
					return fmt.Errorf("timed out waiting for douyin upload button")
				case <-ticker.C:
				}
			}
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
		browser.PasteContent(`div[contenteditable="true"]`, content, false),
		chromedp.Sleep(10*time.Second),

		// 6. 点击底部发布按钮
		chromedp.ActionFunc(func(ctx context.Context) error {
			script := `
				(function() {
					window.scrollTo({ top: document.body.scrollHeight, behavior: "instant" });
					const buttons = Array.from(document.querySelectorAll('button'));
					const publishBtn = buttons.reverse().find((button) => {
						const text = (button.textContent || "").trim();
						return text === "发布";
					});
					if (publishBtn) {
						publishBtn.click();
						return "Publish button clicked";
					}
					return "Publish button NOT found";
				})()
			`
			var res string
			err := chromedp.Evaluate(script, &res).Do(ctx)
			fmt.Printf("Douyin Action: %s\n", res)
			if err == nil && res != "Publish button clicked" {
				return fmt.Errorf("douyin %s", res)
			}
			return err
		}),

		chromedp.Sleep(45*time.Second),
		chromedp.Location(&publishURL),
	)

	if err != nil {
		return "", "", fmt.Errorf("douyin headless publish failed: %w", err)
	}

	return "dy_headless", publishURL, nil
}

func extractDouyinText(raw []byte) string {
	var structured core.AdaptedContent
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

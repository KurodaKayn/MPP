package publisher

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

type DouyinPublisher struct{}

func (d *DouyinPublisher) ValidateConfig(config []byte) error {
	// Douyin specific configuration validation (e.g. hashtags, publish time)
	return nil
}

func (d *DouyinPublisher) AdaptContent(project *models.Project) ([]byte, error) {
	// Douyin usually takes plain text for description
	return []byte(project.SourceContent), nil
}

func (d *DouyinPublisher) Publish(ctx context.Context, pub *models.ProjectPlatformPublication, account *models.PlatformAccount) (string, string, error) {
	if account == nil {
		return "", "", fmt.Errorf("douyin headless publishing requires an account with cookies")
	}

	title := "重构测试：抖音图文分发"
	content := string(pub.AdaptedContent)
	if content == "" {
		content = "这是来自 Go 后端的自动化抖音图文分发测试。#自动化 #Golang"
	}

	// 使用相对路径，确保在不同环境下都能运行
	localImagePath := filepath.Join("backend", "Assets", "132461906_p0_master1200.jpg")

	// Setup headless browser with account cookies
	browserCtx, cancel := SetupBrowser(ctx, account.Cookies)
	defer cancel()

	publishCtx, cancelPublish := context.WithTimeout(browserCtx, 180*time.Second)
	defer cancelPublish()

	var publishURL string
	
	err := chromedp.Run(publishCtx,
		// 1. 进入核心上传页
		chromedp.Navigate("https://creator.douyin.com/creator-micro/content/upload?default-tab=3"),
		chromedp.Sleep(5*time.Second),

		// 2. 根据用户提供的 HTML 源码，精准点击“上传图文”按钮
		chromedp.ActionFunc(func(ctx context.Context) error {
			script := `
				(function() {
					// 优先使用用户提供的精准类名锁定按钮
					const btn = document.querySelector('.container-drag-btn-k6XmB4') || 
					            document.querySelector('button.semi-button-primary');
					if (btn) {
						btn.click();
						return "Target 'Upload Image Post' button clicked via class";
					}
					// 备选：点击整个拖拽容器
					const container = document.querySelector('.container-drag-VAfIfu');
					if (container) {
						container.click();
						return "Drag container clicked as fallback";
					}
					return "Upload button NOT found";
				})()
			`
			var res string
			chromedp.Evaluate(script, &res).Do(ctx)
			fmt.Printf("Douyin Action: %s\n", res)
			return nil
		}),
		chromedp.Sleep(2*time.Second),

		// 3. 注入本地图片 (input[type="file"] 就在你提供的 HTML 底部)
		chromedp.WaitVisible(`input[type="file"]`, chromedp.ByQuery),
		chromedp.SetUploadFiles(`input[type="file"]`, []string{localImagePath}, chromedp.ByQuery),
		chromedp.Sleep(10*time.Second), // 图片上传解析时间


		// 4. 开始填写文字：标题
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

package publisher

import (
	"context"
	"fmt"
	"os"
	"strings"
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

	// 暂时固定图片路径进行测试
	localImagePath := `D:\multi-plantform-poster\backend\Assets\132461906_p0_master1200.jpg`

	// 1. 在 Go 后端读取图片 (确保文件存在)
	info, err := os.Stat(localImagePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to find local image: %w", err)
	}
	_ = info

	// Setup headless browser with account cookies
	browserCtx, cancel := SetupBrowser(ctx, account.Cookies)
	defer cancel()

	publishCtx, cancelPublish := context.WithTimeout(browserCtx, 180*time.Second)
	defer cancelPublish()

	var publishURL string
	var currentURL string

	err = chromedp.Run(publishCtx,
		// 1. 导航到图文发布页 (type=2 代表图文)
		chromedp.Navigate("https://creator.douyin.com/content/publish?type=2"),
		chromedp.Sleep(5*time.Second),
		chromedp.Location(&currentURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			if strings.Contains(currentURL, "login") {
				return fmt.Errorf("抖音登录失效，请更新 Cookie")
			}
			return nil
		}),

		// 2. 上传图片 (利用 input[type="file"])
		// 抖音图文页通常有隐藏的 input 接收图片
		chromedp.WaitVisible(`input[type="file"]`, chromedp.ByQuery),
		chromedp.SetUploadFiles(`input[type="file"]`, []string{localImagePath}, chromedp.ByQuery),
		chromedp.Sleep(10*time.Second), // 等待图片解析和上传进度

		// 3. 填入标题
		// 抖音标题选择器 (需根据实际页面微调)
		chromedp.WaitVisible(`input[placeholder*="标题"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[placeholder*="标题"]`, title),

		// 4. 填入描述正文 (使用模拟粘贴 Ctrl+V 方案，最稳妥)
		// 抖音描述框通常是 contenteditable 的 div
		chromedp.ActionFunc(func(ctx context.Context) error {
			// 寻找描述框并聚焦
			script := `
				(function() {
					const el = document.querySelector('.public-DraftEditor-content') || 
					           document.querySelector('div[contenteditable="true"]');
					if (el) {
						el.focus();
						return "Description editor focused";
					}
					return "Description editor NOT found";
				})()
			`
			return chromedp.Evaluate(script, nil).Do(ctx)
		}),
		PasteContent(`div[contenteditable="true"]`, content, false),
		chromedp.Sleep(2*time.Second),

		// 5. 点击发布
		// 寻找文本为“发布”的按钮
		chromedp.ActionFunc(func(ctx context.Context) error {
			script := `
				(function() {
					const buttons = Array.from(document.querySelectorAll('button'));
					const pubBtn = buttons.find(b => b.textContent.trim() === '发布');
					if (pubBtn) {
						pubBtn.click();
						return "Publish button clicked";
					}
					return "Publish button NOT found";
				})()
			`
			return chromedp.Evaluate(script, nil).Do(ctx)
		}),

		// 6. 等待发布成功跳转
		chromedp.Sleep(20*time.Second),
		chromedp.Location(&publishURL),
	)

	if err != nil {
		return "", "", fmt.Errorf("douyin headless publish failed: %w", err)
	}

	return "dy_headless", publishURL, nil
}

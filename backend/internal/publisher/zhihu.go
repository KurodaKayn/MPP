package publisher

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

type ZhihuPublisher struct{}

func (z *ZhihuPublisher) ValidateConfig(config []byte) error {
	return nil
}

func (z *ZhihuPublisher) AdaptContent(project *models.Project) ([]byte, error) {
	return []byte(fmt.Sprintf(`{"markdown": "%s"}`, project.SourceContent)), nil
}

func (z *ZhihuPublisher) Publish(ctx context.Context, pub *models.ProjectPlatformPublication, account *models.PlatformAccount) (string, string, error) {
	if account == nil {
		return "", "", fmt.Errorf("account information is required for headless publishing")
	}

	title := "重构测试：模拟 Ctrl+V 直接粘贴图片"
	content := "这是一段长正文内容，用于测试模拟 Ctrl+V 直接粘贴本地图片的功能。这种方式不需要点开繁琐的工具栏和浮窗，而是像人类操作一样，将图片数据注入剪贴板并派发粘贴事件。"
	
	localImagePath := `D:\multi-plantform-poster\backend\Assets\132461906_p0_master1200.jpg`

	// 1. 在 Go 后端读取图片并转为 Base64
	imgData, err := os.ReadFile(localImagePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read local image: %w", err)
	}
	imgBase64 := base64.StdEncoding.EncodeToString(imgData)

	browserCtx, cancel := SetupBrowser(ctx, account.Cookies)
	defer cancel()

	publishCtx, cancelPublish := context.WithTimeout(browserCtx, 150*time.Second) 
	defer cancelPublish()

	var publishURL string
	var currentURL string
	
	err = chromedp.Run(publishCtx,
		chromedp.Navigate("https://zhuanlan.zhihu.com/write"),
		chromedp.Sleep(5*time.Second),
		chromedp.Location(&currentURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			if strings.Contains(currentURL, "signin") {
				return fmt.Errorf("登录失效，请更新 Cookie")
			}
			return nil
		}),
		
		// 1. 填标题
		WaitForElement(`textarea[placeholder*="标题"]`, 30*time.Second),
		chromedp.SendKeys(`textarea[placeholder*="标题"]`, title),
		
		// 2. 聚焦并填充文本
		chromedp.Click(`div[data-contents="true"]`, chromedp.ByQuery),
		PasteContent(`div[data-contents="true"]`, content, false),
		chromedp.Sleep(2*time.Second),

		// 3. 核心步骤：模拟 Ctrl+V 粘贴本地图片
		// 我们将图片以 image/jpeg 格式“粘贴”进编辑器
		PasteFile(`div[data-contents="true"]`, "test_image.jpg", "image/jpeg", imgBase64),
		
		chromedp.Sleep(10*time.Second), // 等待图片上传完成

		// 4. 点击发布
		chromedp.Click(`//button[contains(text(), "发布")]`, chromedp.BySearch),
		
		// 增加等待时间到 20 秒，确保知乎后端处理完发布并完成页面跳转
		// 避免过早关闭浏览器导致发布请求被中断（留在草稿箱）
		chromedp.Sleep(20*time.Second), 
		
		chromedp.Location(&publishURL),
	)

	if err != nil {
		return "", "", fmt.Errorf("zhihu publish failed: %w", err)
	}

	return "zh_headless", publishURL, nil
}

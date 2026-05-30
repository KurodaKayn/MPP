package publisher

import (
	"context"
	"encoding/json"
	"fmt"
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
	markdown, err := htmlToMarkdown(project.SourceContent)
	if err != nil {
		return nil, err
	}
	content := systemAdaptedContent(
		project,
		"markdown",
		"zhihu-markdown-adapter",
		htmlToText(project.SourceContent),
	)
	content.Markdown = markdown
	return json.Marshal(content)
}

func (z *ZhihuPublisher) Publish(ctx context.Context, pub *models.ProjectPlatformPublication, account *models.PlatformAccount) (string, string, error) {
	if account == nil {
		return "", "", fmt.Errorf("account information is required for headless publishing")
	}

	title := extractPublicationTitle(pub.Config)
	content := extractZhihuMarkdown(pub.AdaptedContent)
	if content == "" {
		return "", "", fmt.Errorf("zhihu markdown content is empty")
	}

	browserCtx, cancel := SetupBrowser(ctx, account.Cookies)
	defer cancel()

	publishCtx, cancelPublish := context.WithTimeout(browserCtx, 150*time.Second)
	defer cancelPublish()

	var publishURL string
	var currentURL string

	err := chromedp.Run(publishCtx,
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

func extractZhihuMarkdown(raw []byte) string {
	var structured struct {
		Markdown string `json:"markdown"`
		Summary  string `json:"summary"`
	}
	if err := json.Unmarshal(raw, &structured); err == nil {
		if markdown := strings.TrimSpace(structured.Markdown); markdown != "" {
			return markdown
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

func extractPublicationTitle(raw []byte) string {
	var config struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return ""
	}
	return strings.TrimSpace(config.Title)
}

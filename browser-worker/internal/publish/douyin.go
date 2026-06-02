package publish

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/kurodakayn/mpp-browser-worker/internal/session"
)

type DouyinDraftRequest struct {
	Title            string `json:"title"`
	Content          string `json:"content"`
	CoverImageBase64 string `json:"cover_image_base64"`
	CoverImageName   string `json:"cover_image_name"`
}

func RunDouyinDraft(ctx context.Context, workerSession *session.WorkerSession, req DouyinDraftRequest) error {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "抖音图文"
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return fmt.Errorf("douyin text content is empty")
	}

	imagePath, cleanup, err := writeCoverImage(req.CoverImageBase64, req.CoverImageName)
	if err != nil {
		return err
	}
	defer cleanup()

	saveState(ctx, workerSession, "正在打开抖音上传页")

	workerSession.CDPMu.Lock()
	defer workerSession.CDPMu.Unlock()

	runCtx, cancel := context.WithTimeout(workerSession.BrowserContext, 300*time.Second)
	defer cancel()

	if err := chromedp.Run(runCtx, douyinDraftActions(title, content, imagePath)...); err != nil {
		saveState(context.Background(), workerSession, fmt.Sprintf("抖音脚本执行失败: %v", err))
		return fmt.Errorf("douyin visual publish failed: %w", err)
	}

	saveState(context.Background(), workerSession, "抖音图文已点击发布，请在远程浏览器中完成可能出现的验证")
	return nil
}

func douyinDraftActions(title, content, imagePath string) []chromedp.Action {
	return []chromedp.Action{
		chromedp.Navigate("https://creator.douyin.com/creator-micro/content/upload?default-tab=3"),
		waitAndClickDouyinUploadButton(120 * time.Second),
		chromedp.Sleep(2 * time.Second),
		chromedp.WaitReady(`input[type="file"]`, chromedp.ByQuery),
		chromedp.SetUploadFiles(`input[type="file"]`, []string{imagePath}, chromedp.ByQuery),
		chromedp.Sleep(10 * time.Second),
		chromedp.WaitVisible(`input[placeholder*="标题"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[placeholder*="标题"]`, title),
		chromedp.ActionFunc(func(ctx context.Context) error {
			script := fmt.Sprintf(`
				(function() {
					const value = %s;
					const el = document.querySelector('.public-DraftEditor-content') ||
						document.querySelector('div[contenteditable="true"]');
					if (!el) {
						throw new Error("Douyin description editor not found");
					}
					el.focus();
					document.execCommand('selectAll', false, null);
					document.execCommand('insertText', false, value);
				})()
			`, jsonString(content))
			return chromedp.Evaluate(script, nil).Do(ctx)
		}),
		chromedp.Sleep(10 * time.Second),
		clickDouyinPublishButton(),
		chromedp.Sleep(45 * time.Second),
	}
}

func waitAndClickDouyinUploadButton(timeout time.Duration) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		waitCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			var result string
			err := chromedp.Evaluate(`
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
			`, &result).Do(waitCtx)
			if err != nil {
				return err
			}
			if result == "clicked" {
				return nil
			}

			select {
			case <-waitCtx.Done():
				return fmt.Errorf("timed out waiting for douyin upload button")
			case <-ticker.C:
			}
		}
	})
}

func clickDouyinPublishButton() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		var result string
		err := chromedp.Evaluate(`
			(function() {
				window.scrollTo({ top: document.body.scrollHeight, behavior: "instant" });
				const buttons = Array.from(document.querySelectorAll('button'));
				const publishBtn = buttons.reverse().find((button) => {
					const text = (button.textContent || "").trim();
					return text === "发布";
				});
				if (!publishBtn) {
					return "publish button not found";
				}
				publishBtn.click();
				return "publish button clicked";
			})()
		`, &result).Do(ctx)
		if err != nil {
			return err
		}
		if result != "publish button clicked" {
			return fmt.Errorf("douyin %s", result)
		}
		return nil
	})
}

func writeCoverImage(encoded, name string) (string, func(), error) {
	if strings.TrimSpace(encoded) == "" {
		return "", nil, fmt.Errorf("douyin image publish requires a cover image")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", nil, fmt.Errorf("failed to decode douyin cover image: %w", err)
	}
	pattern := "mpp-douyin-cover-*"
	if ext := coverImageExt(name); ext != "" {
		pattern += ext
	}
	file, err := os.CreateTemp("", pattern)
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

func coverImageExt(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		if strings.HasSuffix(lower, ext) {
			return ext
		}
	}
	return ".jpg"
}

func jsonString(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func saveState(ctx context.Context, workerSession *session.WorkerSession, message string) {
	if workerSession.StateStore == nil {
		return
	}
	_ = workerSession.StateStore.SaveLiveSession(ctx, workerSession, session.WorkerSessionState{
		WorkerSessionRef: workerSession.ID,
		Status:           "ready",
		Message:          message,
		ExpiresAt:        workerSession.ExpiresAt,
	})
}

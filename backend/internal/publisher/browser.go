package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// BrowserAction represents a reusable browser automation step
type BrowserAction func(ctx context.Context) error

// SetupBrowser initializes a chromedp context with optional cookies
func SetupBrowser(ctx context.Context, cookiesJSON []byte) (context.Context, context.CancelFunc) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		// Force a standard User-Agent to ensure cookies remain valid across environments
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"),
	)

	// Use CHROME_BIN environment variable if provided, otherwise let chromedp find the browser
	if browserPath := os.Getenv("CHROME_BIN"); browserPath != "" {
		opts = append(opts, chromedp.ExecPath(browserPath))
	} else if browserPath := os.Getenv("BROWSER_BIN"); browserPath != "" {
		opts = append(opts, chromedp.ExecPath(browserPath))
	}

	// Disable headless mode for visual debugging if HEADLESS=false is set
	if os.Getenv("HEADLESS") == "false" {
		opts = append(opts, chromedp.Flag("headless", false))
	}

	allocCtx, _ := chromedp.NewExecAllocator(ctx, opts...)

	ctx, cancel := chromedp.NewContext(allocCtx)

	// Set cookies if provided
	if len(cookiesJSON) > 0 {
		var cookies []Cookie
		if err := json.Unmarshal(cookiesJSON, &cookies); err == nil {
			fmt.Printf("Attempting to set %d cookies...\n", len(cookies))
			
			// Find a representative domain to "anchor" the browser
			// We prioritize zhuanlan.zhihu.com for writing articles
			targetDomain := "https://www.zhihu.com/robots.txt"
			for _, c := range cookies {
				if strings.Contains(c.Domain, "zhuanlan.zhihu.com") {
					targetDomain = "https://zhuanlan.zhihu.com/robots.txt"
					break
				}
			}

			// We navigate to a tiny asset on the target domain first to establish the origin
			// This ensures the browser accepts domain-specific cookies.
			err := chromedp.Run(ctx,
				network.Enable(),
				chromedp.Navigate(targetDomain),
				chromedp.ActionFunc(func(ctx context.Context) error {
					for _, c := range cookies {
						expr := network.SetCookie(c.Name, c.Value).
							WithDomain(c.Domain).
							WithPath(c.Path).
							WithHTTPOnly(c.HttpOnly).
							WithSecure(c.Secure)

						if c.Expires > 0 {
							t := cdp.TimeSinceEpoch(time.Unix(int64(c.Expires), 0))
							expr = expr.WithExpires(&t)
						}

						if err := expr.Do(ctx); err != nil {
							return err
						}
					}
					return nil
				}),
			)
			if err != nil {
				fmt.Printf("Warning: failed to set cookies: %v\n", err)
			}
		}
	}

	return ctx, cancel
}
type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	Secure   bool    `json:"secure"`
	HttpOnly bool    `json:"httpOnly"`
	SameSite string  `json:"sameSite"`
}

func setCookiesAction(cookies []Cookie) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		for _, c := range cookies {
			expr := network.SetCookie(c.Name, c.Value).
				WithDomain(c.Domain).
				WithPath(c.Path).
				WithHTTPOnly(c.HttpOnly).
				WithSecure(c.Secure)

			if c.Expires > 0 {
				t := cdp.TimeSinceEpoch(time.Unix(int64(c.Expires), 0))
				expr = expr.WithExpires(&t)
			}

			if err := expr.Do(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

// WaitForElement is a helper similar to the one in the extension
func WaitForElement(selector string, timeout time.Duration) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return chromedp.WaitVisible(selector, chromedp.ByQuery).Do(timeoutCtx)
	})
}

// PasteFile simulates a human-like Ctrl+V of a file into a focused element
func PasteFile(selector string, fileName string, mimeType string, base64Data string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		script := fmt.Sprintf(`
			(function() {
				const el = document.querySelector('%s');
				if (!el) return "Element not found";
				el.focus();

				// 将 Base64 转换为 Blob
				const byteString = atob('%s');
				const ab = new ArrayBuffer(byteString.length);
				const ia = new Uint8Array(ab);
				for (let i = 0; i < byteString.length; i++) {
					ia[i] = byteString.charCodeAt(i);
				}
				const blob = new Blob([ab], {type: '%s'});
				const file = new File([blob], '%s', { type: '%s', lastModified: Date.now() });

				// 构造 DataTransfer
				const dt = new DataTransfer();
				dt.items.add(file);
				
				// 深度模拟：有些编辑器会检查 .files 属性
				Object.defineProperty(dt, 'files', {
					value: [file],
					writable: false,
					configurable: true
				});

				// 构造并派发 paste 事件
				const event = new Event('paste', { bubbles: true, cancelable: true });
				Object.defineProperty(event, 'clipboardData', { 
					value: dt,
					writable: false,
					configurable: true
				});
				
				el.dispatchEvent(event);
				return "Paste simulated for: " + file.name;
			})()
		`, selector, base64Data, mimeType, fileName, mimeType)
		var res string
		return chromedp.Evaluate(script, &res).Do(ctx)
	})
}
// PasteContent simulates a paste event into an editor
func PasteContent(selector string, content string, isHTML bool) chromedp.Action {
	dataType := "text/plain"
	if isHTML {
		dataType = "text/html"
	}

	return chromedp.ActionFunc(func(ctx context.Context) error {
		script := fmt.Sprintf(`
			(function() {
				const el = document.querySelector('%s');
				if (!el) return;
				el.focus();
				const dataTransfer = new DataTransfer();
				dataTransfer.setData('%s', %q);
				const event = new ClipboardEvent('paste', {
					bubbles: true,
					cancelable: true,
					clipboardData: dataTransfer
				});
				el.dispatchEvent(event);
			})()
		`, selector, dataType, content)
		return chromedp.Evaluate(script, nil).Do(ctx)
	})
}

package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
	"github.com/kurodakayn/mpp-browser-worker/internal/session"
)

func VersionWebSocketURL(cdpPort int) (string, error) {
	reqURL := fmt.Sprintf("http://127.0.0.1:%d/json/version", cdpPort)
	client := &http.Client{Timeout: 2 * time.Second}

	for i := 0; i < 10; i++ {
		httpReq, _ := http.NewRequest(http.MethodGet, reqURL, nil)
		httpReq.Host = "localhost" // Bypass Chromium Host check

		resp, err := client.Do(httpReq)
		if err == nil && resp.StatusCode == http.StatusOK {
			var result struct {
				WebSocketDebuggerUrl string `json:"webSocketDebuggerUrl"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && result.WebSocketDebuggerUrl != "" {
				u, _ := url.Parse(result.WebSocketDebuggerUrl)
				u.Host = fmt.Sprintf("127.0.0.1:%d", cdpPort)
				resp.Body.Close()
				return u.String(), nil
			}
			resp.Body.Close()
		} else if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}

	return "", fmt.Errorf("no browser version websocket target found on CDP port %d", cdpPort)
}

func browserWebSocketURL(cdpPort int) (string, error) {
	reqURL := fmt.Sprintf("http://127.0.0.1:%d/json", cdpPort)
	client := &http.Client{Timeout: 5 * time.Second}

	for i := 0; i < 5; i++ {
		httpReq, _ := http.NewRequest(http.MethodGet, reqURL, nil)
		httpReq.Host = "localhost" // Bypass Host validation

		resp, err := client.Do(httpReq)
		if err != nil {
			log.Printf("CDP target check error: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		func() {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return
			}

			var targets []struct {
				Type                 string `json:"type"`
				WebSocketDebuggerUrl string `json:"webSocketDebuggerUrl"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
				return
			}
			for _, t := range targets {
				if t.Type == "page" && t.WebSocketDebuggerUrl != "" {
					u, _ := url.Parse(t.WebSocketDebuggerUrl)
					u.Host = fmt.Sprintf("127.0.0.1:%d", cdpPort)
					reqURL = u.String()
					break
				}
			}
		}()

		if strings.HasPrefix(reqURL, "ws://") {
			return reqURL, nil
		}
		time.Sleep(1 * time.Second)
	}

	return "", fmt.Errorf("no page websocket target found on CDP port %d", cdpPort)
}

func Snapshot(ctx context.Context, workerSession *session.WorkerSession, includeAccount bool) (string, []session.Cookie, string, error) {
	workerSession.CDPMu.Lock()
	defer workerSession.CDPMu.Unlock()

	var currentURL string
	var cookies []session.Cookie
	var username string

	actions := []chromedp.Action{
		chromedp.Location(&currentURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			chromeCookies, err := storage.GetCookies().Do(ctx)
			if err != nil {
				return err
			}
			for _, cc := range chromeCookies {
				cookies = append(cookies, session.Cookie{
					Name:     cc.Name,
					Value:    cc.Value,
					Domain:   cc.Domain,
					Path:     cc.Path,
					Expires:  cc.Expires,
					Secure:   cc.Secure,
					HttpOnly: cc.HTTPOnly,
				})
			}
			return nil
		}),
	}
	if includeAccount {
		actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
			script := `(function() {
				const nameEl = document.querySelector('.name-G1vOOn') ||
				               document.querySelector('.user-name') ||
				               document.querySelector('[class*="user-name"]') ||
				               document.querySelector('.AppHeader-profileAvatar') ||
				               document.querySelector('.ProfileHeader-name');
				return nameEl ? (nameEl.alt || nameEl.innerText || "") : "";
			})()`
			_ = chromedp.Evaluate(script, &username).Do(ctx)
			return nil
		}))
	}

	runCtx, cancel := context.WithCancel(workerSession.BrowserContext)
	defer cancel()
	stopCallerCancel := context.AfterFunc(ctx, cancel)
	defer stopCallerCancel()

	if err := chromedp.Run(runCtx, actions...); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", nil, "", ctxErr
		}
		return "", nil, "", err
	}

	return currentURL, cookies, username, nil
}

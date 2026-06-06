package isolation

import (
	"context"
	"fmt"
	"log"
	"net/url"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"github.com/kurodakayn/mpp-browser-worker/internal/session"
)

func SetupInterception(ctx context.Context, rules []session.DomainRule) error {
	// Enable fetch interception
	err := chromedp.Run(ctx, fetch.Enable().WithPatterns([]*fetch.RequestPattern{
		{RequestStage: fetch.RequestStageRequest},
	}))
	if err != nil {
		return fmt.Errorf("failed to enable fetch interception: %w", err)
	}

	chromedp.ListenTarget(ctx, func(ev any) {
		switch ev := ev.(type) {
		case *fetch.EventRequestPaused:
			go func() {
				if IsDomainAllowed(ev.Request.URL, rules) {
					// Continue the request
					err := chromedp.Run(ctx, fetch.ContinueRequest(ev.RequestID))
					if err != nil {
						log.Printf("Failed to continue request %s: %v", safeRequestURL(ev.Request.URL), err)
					}
				} else {
					// Block the request
					log.Printf("BLOCKING unauthorized request: %s", safeRequestURL(ev.Request.URL))
					err := chromedp.Run(ctx, fetch.FailRequest(ev.RequestID, network.ErrorReasonAccessDenied))
					if err != nil {
						log.Printf("Failed to fail request %s: %v", safeRequestURL(ev.Request.URL), err)
					}
				}
			}()
		}
	})

	return nil
}

func safeRequestURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "<invalid-url>"
	}
	return u.Scheme + "://" + u.Host + u.EscapedPath()
}

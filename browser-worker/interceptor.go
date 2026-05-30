package main

import (
	"context"
	"fmt"
	"log"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func SetupInterception(ctx context.Context, rules []DomainRule) error {
	// Enable fetch interception
	err := chromedp.Run(ctx, fetch.Enable().WithPatterns([]*fetch.RequestPattern{
		{RequestStage: fetch.RequestStageRequest},
	}))
	if err != nil {
		return fmt.Errorf("failed to enable fetch interception: %w", err)
	}

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *fetch.EventRequestPaused:
			go func() {
				// Create a new background context for the async action
				// We use the root context but handle it carefully
				if IsDomainAllowed(ev.Request.URL, rules) {
					// Continue the request
					err := chromedp.Run(ctx, fetch.ContinueRequest(ev.RequestID))
					if err != nil {
						log.Printf("Failed to continue request %s: %v", ev.Request.URL, err)
					}
				} else {
					// Block the request
					log.Printf("BLOCKING unauthorized request: %s", ev.Request.URL)
					err := chromedp.Run(ctx, fetch.FailRequest(ev.RequestID, network.ErrorReasonAccessDenied))
					if err != nil {
						log.Printf("Failed to fail request %s: %v", ev.Request.URL, err)
					}
				}
			}()
		}
	})

	return nil
}

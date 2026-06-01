package publisher

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/require"
)

func TestSetupBrowserRestoresCookiesWithoutNavigationPreflight(t *testing.T) {
	oldRunBrowserActions := runBrowserActions
	var capturedActions []chromedp.Action
	runBrowserActions = func(ctx context.Context, actions ...chromedp.Action) error {
		capturedActions = append([]chromedp.Action(nil), actions...)
		return nil
	}
	t.Cleanup(func() {
		runBrowserActions = oldRunBrowserActions
	})

	cookiesJSON, err := json.Marshal([]Cookie{
		{
			Name:     "sessionid",
			Value:    "secret-value",
			Domain:   ".douyin.com",
			Path:     "/",
			Secure:   true,
			HttpOnly: true,
		},
	})
	require.NoError(t, err)

	_, cancel := SetupBrowser(context.Background(), "", cookiesJSON)
	cancel()

	require.Len(t, capturedActions, 2)
	require.IsType(t, network.Enable(), capturedActions[0])
	_, ok := capturedActions[1].(chromedp.ActionFunc)
	require.True(t, ok)
}

package browser

import (
	"context"
	"encoding/json"
	"reflect"
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

func TestLocalBrowserAllocatorOptionsAddsNoSandboxForRoot(t *testing.T) {
	opts := localBrowserAllocatorOptions("", true)

	require.True(t, allocatorFlag(opts, "no-sandbox"))
}

func TestLocalBrowserAllocatorOptionsDoesNotForceNoSandboxForNonRoot(t *testing.T) {
	opts := localBrowserAllocatorOptions("", false)

	require.False(t, allocatorFlag(opts, "no-sandbox"))
}

func TestLocalBrowserAllocatorOptionsUsesConfiguredChromeBin(t *testing.T) {
	const browserPath = "/usr/bin/chromium-browser"

	opts := localBrowserAllocatorOptions(browserPath, false)

	require.Equal(t, browserPath, allocatorExecPath(opts))
}

func allocatorFlag(opts []chromedp.ExecAllocatorOption, flag string) bool {
	ctx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	allocator := chromedp.FromContext(ctx).Allocator
	flags := reflect.ValueOf(allocator).Elem().FieldByName("initFlags")
	value := flags.MapIndex(reflect.ValueOf(flag))
	if value.IsValid() && value.Kind() == reflect.Interface {
		value = value.Elem()
	}

	return value.IsValid() && value.Kind() == reflect.Bool && value.Bool()
}

func allocatorExecPath(opts []chromedp.ExecAllocatorOption) string {
	ctx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	allocator := chromedp.FromContext(ctx).Allocator
	return reflect.ValueOf(allocator).Elem().FieldByName("execPath").String()
}

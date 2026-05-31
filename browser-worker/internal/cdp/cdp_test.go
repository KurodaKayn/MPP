package cdp

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/chromedp/cdproto/target"
	"github.com/stretchr/testify/require"
)

func TestPageTargetIDPrefersInitialBlankPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/json", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[
			{"id":"login-page","type":"page","url":"https://creator.douyin.com/creator-micro/home"},
			{"id":"visible-page","type":"page","url":"about:blank"}
		]`)
	}))
	defer server.Close()

	host, rawPort, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(rawPort)
	require.NoError(t, err)

	targetID, err := PageTargetID(host, port)
	require.NoError(t, err)
	require.Equal(t, target.ID("visible-page"), targetID)
}

package app

import (
	"os"
	"strings"

	"github.com/kurodakayn/mpp-backend/internal/publisher"
)

const browserWorkerInternalTokenEnv = "BROWSER_WORKER_INTERNAL_TOKEN"

func NewBrowserWorkerClientFromEnv() publisher.BrowserWorkerClient {
	workerURL := strings.TrimSpace(os.Getenv("BROWSER_WORKER_URL"))
	if workerURL == "" {
		return publisher.NewMockBrowserWorkerClient()
	}
	internalToken := strings.TrimSpace(os.Getenv(browserWorkerInternalTokenEnv))
	return publisher.NewHTTPBrowserWorkerClientWithToken(workerURL, internalToken)
}

package app

import (
	"os"
	"strings"

	"github.com/kurodakayn/mpp-backend/internal/publisher"
)

func NewBrowserWorkerClientFromEnv() publisher.BrowserWorkerClient {
	workerURL := strings.TrimSpace(os.Getenv("BROWSER_WORKER_URL"))
	if workerURL == "" {
		return publisher.NewMockBrowserWorkerClient()
	}
	return publisher.NewHTTPBrowserWorkerClient(workerURL)
}

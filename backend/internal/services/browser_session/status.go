package browsersession

import (
	"github.com/kurodakayn/mpp-backend/internal/models"
)

func isStreamableBrowserSessionStatus(status string) bool {
	switch status {
	case models.BrowserSessionStatusReady,
		models.BrowserSessionStatusLoginDetected,
		models.BrowserSessionStatusCapturing:
		return true
	default:
		return false
	}
}

func isTerminalBrowserSessionStatus(status string) bool {
	switch status {
	case models.BrowserSessionStatusConnected,
		models.BrowserSessionStatusExpired,
		models.BrowserSessionStatusFailed:
		return true
	default:
		return false
	}
}

func activeBrowserSessionStatuses() []string {
	return []string{
		models.BrowserSessionStatusPending,
		models.BrowserSessionStatusReady,
		models.BrowserSessionStatusLoginDetected,
		models.BrowserSessionStatusCapturing,
	}
}

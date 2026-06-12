package media

import "strings"

// DownloadAndProcess delegates platform media handling to content-pipeline-service.
func DownloadAndProcess(sourceURL string) ([]byte, error) {
	return DownloadAndProcessForPlatform(sourceURL, "wechat", "inline_image")
}

func DownloadAndProcessForPlatform(sourceURL string, platform string, usage string) ([]byte, error) {
	return processWithContentPipeline(sourceURL, platform, usage)
}

func isMediaObjectRef(sourceURL string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(sourceURL)), mediaObjectRefPrefix)
}

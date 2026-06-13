package media

import "strings"

// DownloadAndProcess delegates platform media handling to content-pipeline-service.
func DownloadAndProcess(sourceURL string) (string, error) {
	return DownloadAndProcessForPlatform(sourceURL, "wechat", "inline_image")
}

func DownloadAndProcessForPlatform(sourceURL string, platform string, usage string) (string, error) {
	return processWithContentPipeline(sourceURL, platform, usage)
}

func isMediaObjectRef(sourceURL string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(sourceURL)), mediaObjectRefPrefix)
}

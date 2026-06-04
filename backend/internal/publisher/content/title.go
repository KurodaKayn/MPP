package content

import (
	"encoding/json"
	"strings"
)

func ExtractPublicationTitle(raw []byte) string {
	var config struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return ""
	}
	return strings.TrimSpace(config.Title)
}

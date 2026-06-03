package envutil

import (
	"os"
	"strconv"
	"strings"
	"time"
)

func Bool(name string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func Duration(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	if value, err := time.ParseDuration(raw); err == nil && value >= 0 {
		return value
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seconds < 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

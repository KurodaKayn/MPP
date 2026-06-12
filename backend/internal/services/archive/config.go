package archive

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	eventArchiveEnabledEnv                    = "EVENT_ARCHIVE_ENABLED"
	eventArchiveIntervalEnv                   = "EVENT_ARCHIVE_INTERVAL"
	eventArchiveBatchSizeEnv                  = "EVENT_ARCHIVE_BATCH_SIZE"
	eventArchiveObjectPrefixEnv               = "EVENT_ARCHIVE_OBJECT_PREFIX"
	publishEventRetentionDaysEnv              = "PUBLISH_EVENT_RETENTION_DAYS"
	extensionExecutionEventRetentionDaysEnv   = "EXTENSION_EXECUTION_EVENT_RETENTION_DAYS"
	projectActivityRetentionDaysEnv           = "PROJECT_ACTIVITY_RETENTION_DAYS"
	workspaceActivityRetentionDaysEnv         = "WORKSPACE_ACTIVITY_RETENTION_DAYS"
	browserSessionHistoryRetentionDaysEnv     = "BROWSER_SESSION_HISTORY_RETENTION_DAYS"
	defaultArchiveInterval                    = 24 * time.Hour
	defaultArchiveBatchSize                   = 500
	defaultArchiveObjectPrefix                = "archives/database"
	defaultPublishEventRetentionDays          = 180
	defaultExtensionExecutionEventRetention   = 180
	defaultProjectActivityRetentionDays       = 365
	defaultWorkspaceActivityRetentionDays     = 365
	defaultBrowserSessionHistoryRetentionDays = 90
)

// Config controls row-level cold event and session-history archival.
type Config struct {
	Enabled                          bool
	Interval                         time.Duration
	BatchSize                        int
	ObjectKeyPrefix                  string
	PublishEventRetention            time.Duration
	ExtensionExecutionEventRetention time.Duration
	ProjectActivityRetention         time.Duration
	WorkspaceActivityRetention       time.Duration
	BrowserSessionHistoryRetention   time.Duration
}

// ConfigFromEnv reads archival policy and worker settings from environment variables.
func ConfigFromEnv() (Config, error) {
	interval, err := durationFromEnv(eventArchiveIntervalEnv, defaultArchiveInterval)
	if err != nil {
		return Config{}, err
	}
	batchSize, err := positiveIntFromEnv(eventArchiveBatchSizeEnv, defaultArchiveBatchSize)
	if err != nil {
		return Config{}, err
	}
	publishRetention, err := retentionDaysFromEnv(publishEventRetentionDaysEnv, defaultPublishEventRetentionDays)
	if err != nil {
		return Config{}, err
	}
	extensionRetention, err := retentionDaysFromEnv(extensionExecutionEventRetentionDaysEnv, defaultExtensionExecutionEventRetention)
	if err != nil {
		return Config{}, err
	}
	projectActivityRetention, err := retentionDaysFromEnv(projectActivityRetentionDaysEnv, defaultProjectActivityRetentionDays)
	if err != nil {
		return Config{}, err
	}
	workspaceActivityRetention, err := retentionDaysFromEnv(workspaceActivityRetentionDaysEnv, defaultWorkspaceActivityRetentionDays)
	if err != nil {
		return Config{}, err
	}
	sessionRetention, err := retentionDaysFromEnv(browserSessionHistoryRetentionDaysEnv, defaultBrowserSessionHistoryRetentionDays)
	if err != nil {
		return Config{}, err
	}

	prefix := strings.Trim(strings.TrimSpace(os.Getenv(eventArchiveObjectPrefixEnv)), "/")
	if prefix == "" {
		prefix = defaultArchiveObjectPrefix
	}

	return Config{
		Enabled:                          boolFromEnv(eventArchiveEnabledEnv, false),
		Interval:                         interval,
		BatchSize:                        batchSize,
		ObjectKeyPrefix:                  prefix,
		PublishEventRetention:            publishRetention,
		ExtensionExecutionEventRetention: extensionRetention,
		ProjectActivityRetention:         projectActivityRetention,
		WorkspaceActivityRetention:       workspaceActivityRetention,
		BrowserSessionHistoryRetention:   sessionRetention,
	}, nil
}

func boolFromEnv(name string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func durationFromEnv(name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", name, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}
	return value, nil
}

func positiveIntFromEnv(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a positive integer: %w", name, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}
	return value, nil
}

func retentionDaysFromEnv(name string, fallback int) (time.Duration, error) {
	days, err := positiveIntFromEnv(name, fallback)
	if err != nil {
		return 0, err
	}
	return time.Duration(days) * 24 * time.Hour, nil
}

package archive

import (
	"testing"
	"time"
)

func TestConfigFromEnvDefaultsToDisabledRetentionPolicy(t *testing.T) {
	config, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("expected default archive config: %v", err)
	}

	if config.Enabled {
		t.Fatal("archive worker should be disabled by default")
	}
	if config.Interval != 24*time.Hour {
		t.Fatalf("expected daily interval, got %s", config.Interval)
	}
	if config.BatchSize != 500 {
		t.Fatalf("expected default batch size 500, got %d", config.BatchSize)
	}
	if config.ObjectKeyPrefix != "archives/database" {
		t.Fatalf("expected default object key prefix, got %q", config.ObjectKeyPrefix)
	}
	if config.PublishEventRetention != 180*24*time.Hour {
		t.Fatalf("expected publish event retention to be 180 days, got %s", config.PublishEventRetention)
	}
	if config.ExtensionExecutionEventRetention != 180*24*time.Hour {
		t.Fatalf("expected extension event retention to be 180 days, got %s", config.ExtensionExecutionEventRetention)
	}
	if config.ProjectActivityRetention != 365*24*time.Hour {
		t.Fatalf("expected project activity retention to be 365 days, got %s", config.ProjectActivityRetention)
	}
	if config.WorkspaceActivityRetention != 365*24*time.Hour {
		t.Fatalf("expected workspace activity retention to be 365 days, got %s", config.WorkspaceActivityRetention)
	}
	if config.BrowserSessionHistoryRetention != 90*24*time.Hour {
		t.Fatalf("expected browser session retention to be 90 days, got %s", config.BrowserSessionHistoryRetention)
	}
}

func TestConfigFromEnvReadsOverrides(t *testing.T) {
	t.Setenv(eventArchiveEnabledEnv, "true")
	t.Setenv(eventArchiveIntervalEnv, "2h")
	t.Setenv(eventArchiveBatchSizeEnv, "25")
	t.Setenv(eventArchiveObjectPrefixEnv, "/custom/archive/")
	t.Setenv(publishEventRetentionDaysEnv, "30")
	t.Setenv(extensionExecutionEventRetentionDaysEnv, "31")
	t.Setenv(projectActivityRetentionDaysEnv, "32")
	t.Setenv(workspaceActivityRetentionDaysEnv, "33")
	t.Setenv(browserSessionHistoryRetentionDaysEnv, "34")

	config, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("expected override archive config: %v", err)
	}

	if !config.Enabled {
		t.Fatal("archive worker should read enabled flag")
	}
	if config.Interval != 2*time.Hour {
		t.Fatalf("expected 2h interval, got %s", config.Interval)
	}
	if config.BatchSize != 25 {
		t.Fatalf("expected batch size 25, got %d", config.BatchSize)
	}
	if config.ObjectKeyPrefix != "custom/archive" {
		t.Fatalf("expected trimmed prefix, got %q", config.ObjectKeyPrefix)
	}
	if config.PublishEventRetention != 30*24*time.Hour {
		t.Fatalf("expected publish retention override, got %s", config.PublishEventRetention)
	}
	if config.ExtensionExecutionEventRetention != 31*24*time.Hour {
		t.Fatalf("expected extension retention override, got %s", config.ExtensionExecutionEventRetention)
	}
	if config.ProjectActivityRetention != 32*24*time.Hour {
		t.Fatalf("expected project activity retention override, got %s", config.ProjectActivityRetention)
	}
	if config.WorkspaceActivityRetention != 33*24*time.Hour {
		t.Fatalf("expected workspace activity retention override, got %s", config.WorkspaceActivityRetention)
	}
	if config.BrowserSessionHistoryRetention != 34*24*time.Hour {
		t.Fatalf("expected browser session retention override, got %s", config.BrowserSessionHistoryRetention)
	}
}

func TestConfigFromEnvRejectsInvalidValues(t *testing.T) {
	t.Setenv(eventArchiveBatchSizeEnv, "0")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("expected zero batch size to be rejected")
	}

	t.Setenv(eventArchiveBatchSizeEnv, "10")
	t.Setenv(publishEventRetentionDaysEnv, "bad")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("expected invalid retention days to be rejected")
	}
}

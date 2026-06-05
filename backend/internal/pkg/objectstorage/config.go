package objectstorage

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	objectStorageProviderEnv = "OBJECT_STORAGE_PROVIDER"
	r2AccountIDEnv           = "R2_ACCOUNT_ID"
	r2AccessKeyIDEnv         = "R2_ACCESS_KEY_ID"
	r2SecretAccessKeyEnv     = "R2_SECRET_ACCESS_KEY"
	r2BucketEnv              = "R2_BUCKET"
	r2EndpointEnv            = "R2_ENDPOINT"
	r2RegionEnv              = "R2_REGION"
	mediaUploadURLTTLEnv     = "MEDIA_UPLOAD_URL_TTL"
	mediaDownloadURLTTLEnv   = "MEDIA_DOWNLOAD_URL_TTL"

	defaultR2Region       = "auto"
	defaultUploadURLTTL   = 10 * time.Minute
	defaultDownloadURLTTL = 5 * time.Minute
	r2EndpointTemplate    = "https://%s.r2.cloudflarestorage.com"
)

// ConfigFromEnv reads and validates object storage settings from environment variables.
func ConfigFromEnv() (Config, error) {
	provider := strings.ToLower(envString(objectStorageProviderEnv))
	if provider == "" || provider == ProviderDisabled {
		return Config{
			Enabled:        false,
			Provider:       ProviderDisabled,
			Region:         defaultR2Region,
			UploadURLTTL:   defaultUploadURLTTL,
			DownloadURLTTL: defaultDownloadURLTTL,
		}, nil
	}
	if provider != ProviderR2 {
		return Config{}, fmt.Errorf("%s has unsupported provider %q", objectStorageProviderEnv, provider)
	}

	uploadTTL, err := durationFromEnv(mediaUploadURLTTLEnv, defaultUploadURLTTL)
	if err != nil {
		return Config{}, err
	}
	downloadTTL, err := durationFromEnv(mediaDownloadURLTTLEnv, defaultDownloadURLTTL)
	if err != nil {
		return Config{}, err
	}

	config := Config{
		Enabled:         true,
		Provider:        ProviderR2,
		AccountID:       envString(r2AccountIDEnv),
		AccessKeyID:     envString(r2AccessKeyIDEnv),
		SecretAccessKey: envString(r2SecretAccessKeyEnv),
		Bucket:          envString(r2BucketEnv),
		Endpoint:        strings.TrimRight(envString(r2EndpointEnv), "/"),
		Region:          envString(r2RegionEnv),
		UploadURLTTL:    uploadTTL,
		DownloadURLTTL:  downloadTTL,
	}
	if config.Region == "" {
		config.Region = defaultR2Region
	}
	if config.Endpoint == "" && config.AccountID != "" {
		config.Endpoint = fmt.Sprintf(r2EndpointTemplate, config.AccountID)
	}
	if err := validateR2Config(config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func validateR2Config(config Config) error {
	required := map[string]string{
		r2AccountIDEnv:       config.AccountID,
		r2AccessKeyIDEnv:     config.AccessKeyID,
		r2SecretAccessKeyEnv: config.SecretAccessKey,
		r2BucketEnv:          config.Bucket,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required when %s=%s", name, objectStorageProviderEnv, ProviderR2)
		}
	}
	if strings.TrimSpace(config.Endpoint) == "" {
		return fmt.Errorf("%s or %s is required when %s=%s", r2EndpointEnv, r2AccountIDEnv, objectStorageProviderEnv, ProviderR2)
	}
	if strings.TrimSpace(config.Region) == "" {
		return fmt.Errorf("%s is required when %s=%s", r2RegionEnv, objectStorageProviderEnv, ProviderR2)
	}
	return nil
}

func envString(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

func durationFromEnv(name string, fallback time.Duration) (time.Duration, error) {
	raw := envString(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		seconds, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil {
			return 0, fmt.Errorf("%s must be a duration: %w", name, err)
		}
		value = time.Duration(seconds) * time.Second
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}
	return value, nil
}

package objectstorage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigFromEnvReturnsDisabledByDefault(t *testing.T) {
	clearObjectStorageEnv(t)

	config, err := ConfigFromEnv()

	require.NoError(t, err)
	require.False(t, config.Enabled)
	require.Equal(t, ProviderDisabled, config.Provider)
}

func TestConfigFromEnvBuildsR2Defaults(t *testing.T) {
	clearObjectStorageEnv(t)
	t.Setenv(objectStorageProviderEnv, ProviderR2)
	t.Setenv(r2AccountIDEnv, "account-id")
	t.Setenv(r2AccessKeyIDEnv, "access-key")
	t.Setenv(r2SecretAccessKeyEnv, "secret-key")
	t.Setenv(r2BucketEnv, "mpp-media")

	config, err := ConfigFromEnv()

	require.NoError(t, err)
	require.True(t, config.Enabled)
	require.Equal(t, ProviderR2, config.Provider)
	require.Equal(t, "https://account-id.r2.cloudflarestorage.com", config.Endpoint)
	require.Equal(t, "auto", config.Region)
	require.Equal(t, "mpp-media", config.Bucket)
	require.Equal(t, "access-key", config.AccessKeyID)
	require.Equal(t, "secret-key", config.SecretAccessKey)
	require.Equal(t, 10*time.Minute, config.UploadURLTTL)
	require.Equal(t, 5*time.Minute, config.DownloadURLTTL)
}

func TestConfigFromEnvUsesOverrides(t *testing.T) {
	clearObjectStorageEnv(t)
	t.Setenv(objectStorageProviderEnv, ProviderR2)
	t.Setenv(r2AccountIDEnv, "account-id")
	t.Setenv(r2AccessKeyIDEnv, "access-key")
	t.Setenv(r2SecretAccessKeyEnv, "secret-key")
	t.Setenv(r2BucketEnv, "mpp-media")
	t.Setenv(r2EndpointEnv, " https://r2.internal.example.com/ ")
	t.Setenv(r2RegionEnv, "custom-region")
	t.Setenv(mediaUploadURLTTLEnv, "15m")
	t.Setenv(mediaDownloadURLTTLEnv, "90s")

	config, err := ConfigFromEnv()

	require.NoError(t, err)
	require.Equal(t, "https://r2.internal.example.com", config.Endpoint)
	require.Equal(t, "custom-region", config.Region)
	require.Equal(t, 15*time.Minute, config.UploadURLTTL)
	require.Equal(t, 90*time.Second, config.DownloadURLTTL)
}

func TestConfigFromEnvRequiresR2FieldsWhenEnabled(t *testing.T) {
	required := []string{
		r2AccountIDEnv,
		r2AccessKeyIDEnv,
		r2SecretAccessKeyEnv,
		r2BucketEnv,
	}

	for _, missing := range required {
		t.Run(missing, func(t *testing.T) {
			clearObjectStorageEnv(t)
			t.Setenv(objectStorageProviderEnv, ProviderR2)
			t.Setenv(r2AccountIDEnv, "account-id")
			t.Setenv(r2AccessKeyIDEnv, "access-key")
			t.Setenv(r2SecretAccessKeyEnv, "secret-key")
			t.Setenv(r2BucketEnv, "mpp-media")
			t.Setenv(missing, "")

			_, err := ConfigFromEnv()

			require.Error(t, err)
			require.Contains(t, err.Error(), missing)
		})
	}
}

func TestConfigFromEnvRejectsInvalidTTL(t *testing.T) {
	clearObjectStorageEnv(t)
	t.Setenv(objectStorageProviderEnv, ProviderR2)
	t.Setenv(r2AccountIDEnv, "account-id")
	t.Setenv(r2AccessKeyIDEnv, "access-key")
	t.Setenv(r2SecretAccessKeyEnv, "secret-key")
	t.Setenv(r2BucketEnv, "mpp-media")
	t.Setenv(mediaUploadURLTTLEnv, "-1s")

	_, err := ConfigFromEnv()

	require.Error(t, err)
	require.Contains(t, err.Error(), mediaUploadURLTTLEnv)
}

func clearObjectStorageEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{
		objectStorageProviderEnv,
		r2AccountIDEnv,
		r2AccessKeyIDEnv,
		r2SecretAccessKeyEnv,
		r2BucketEnv,
		r2EndpointEnv,
		r2RegionEnv,
		mediaUploadURLTTLEnv,
		mediaDownloadURLTTLEnv,
	} {
		t.Setenv(name, "")
	}
}

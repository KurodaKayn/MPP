package r2

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
)

func TestClientPresignsR2ObjectURLs(t *testing.T) {
	client, err := NewClient(objectstorage.Config{
		Enabled:         true,
		Provider:        objectstorage.ProviderR2,
		Endpoint:        "https://account-id.r2.cloudflarestorage.com",
		Region:          "auto",
		Bucket:          "mpp-media",
		AccessKeyID:     "access-key",
		SecretAccessKey: "secret-key",
	})
	require.NoError(t, err)

	presigned, err := client.PresignPutObject(context.Background(), objectstorage.PutObjectInput{
		Bucket:      "mpp-media",
		Key:         "projects/asset.png",
		ContentType: "image/png",
		Expires:     10 * time.Minute,
	})
	require.NoError(t, err)

	parsed, err := url.Parse(presigned.URL)
	require.NoError(t, err)
	require.Equal(t, "account-id.r2.cloudflarestorage.com", parsed.Host)
	require.Equal(t, "/mpp-media/projects/asset.png", parsed.Path)
	require.Equal(t, "AWS4-HMAC-SHA256", parsed.Query().Get("X-Amz-Algorithm"))
	require.Equal(t, "access-key", parsed.Query().Get("X-Amz-Credential")[:len("access-key")])
	require.Equal(t, map[string]string{"Content-Type": "image/png"}, presigned.Headers)
	require.Equal(t, 10*time.Minute, presigned.Expires)
}

func TestNewClientRejectsInvalidConfig(t *testing.T) {
	_, err := NewClient(objectstorage.Config{
		Enabled:  true,
		Provider: objectstorage.ProviderR2,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "R2")
}

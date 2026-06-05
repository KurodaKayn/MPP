package fake

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
	"github.com/stretchr/testify/require"
)

func TestClientPresignsObjectURLs(t *testing.T) {
	client := NewClient()

	putURL, err := client.PresignPutObject(context.Background(), objectstorage.PutObjectInput{
		Bucket:      "media",
		Key:         "projects/asset.png",
		ContentType: "image/png",
		Expires:     10 * time.Minute,
	})
	require.NoError(t, err)
	require.Equal(t, "fake://put/media/projects/asset.png", putURL.URL)
	require.Equal(t, 10*time.Minute, putURL.Expires)
	require.Equal(t, map[string]string{"Content-Type": "image/png"}, putURL.Headers)

	getURL, err := client.PresignGetObject(context.Background(), objectstorage.GetObjectInput{
		Bucket:  "media",
		Key:     "projects/asset.png",
		Expires: 5 * time.Minute,
	})
	require.NoError(t, err)
	require.Equal(t, "fake://get/media/projects/asset.png", getURL.URL)
	require.Equal(t, 5*time.Minute, getURL.Expires)
}

func TestClientStoresAndDeletesObjects(t *testing.T) {
	client := NewClient()
	client.StoreObject("projects/asset.png", []byte("image-bytes"), objectstorage.ObjectInfo{
		Key:         "projects/asset.png",
		ContentType: "image/png",
		Size:        11,
		ETag:        "etag-value",
	})

	info, err := client.HeadObject(context.Background(), "projects/asset.png")
	require.NoError(t, err)
	require.Equal(t, "image/png", info.ContentType)
	require.EqualValues(t, 11, info.Size)

	body, objectInfo, err := client.GetObject(context.Background(), "projects/asset.png")
	require.NoError(t, err)
	defer body.Close()
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, []byte("image-bytes"), data)
	require.Equal(t, info, objectInfo)

	require.NoError(t, client.DeleteObject(context.Background(), "projects/asset.png"))
	_, err = client.HeadObject(context.Background(), "projects/asset.png")
	require.ErrorIs(t, err, objectstorage.ErrObjectNotFound)
}

package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadProcessedObjectReadsFilesystemObjectRef(t *testing.T) {
	root := t.TempDir()
	key := "processed/ab/abcdef.png"
	path := filepath.Join(root, filepath.FromSlash(key))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte("image-bytes"), 0o600))
	t.Setenv(contentPipelineMediaObjectStoreEnv, "filesystem")
	t.Setenv(contentPipelineMediaObjectRootEnv, root)

	data, err := ReadProcessedObject(context.Background(), defaultProcessedMediaObjectRefPrefix+key)

	require.NoError(t, err)
	require.Equal(t, []byte("image-bytes"), data)
}

func TestMaterializeProcessedObjectWritesTempFile(t *testing.T) {
	root := t.TempDir()
	key := "processed/ab/abcdef.png"
	path := filepath.Join(root, filepath.FromSlash(key))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte("image-bytes"), 0o600))
	t.Setenv(contentPipelineMediaObjectStoreEnv, "filesystem")
	t.Setenv(contentPipelineMediaObjectRootEnv, root)

	tempPath, cleanup, err := MaterializeProcessedObject(context.Background(), defaultProcessedMediaObjectRefPrefix+key, "mpp-media-test-*")
	require.NoError(t, err)
	defer cleanup()

	data, err := os.ReadFile(tempPath) //nolint:gosec // Path returned by MaterializeProcessedObject.
	require.NoError(t, err)
	require.Equal(t, []byte("image-bytes"), data)
}

func TestReadProcessedObjectRejectsUnsupportedRefs(t *testing.T) {
	t.Setenv(contentPipelineMediaObjectStoreEnv, "filesystem")
	t.Setenv(contentPipelineMediaObjectRootEnv, t.TempDir())

	_, err := ReadProcessedObject(context.Background(), "mpp://media/11111111-1111-4111-8111-111111111111")

	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported processed media object ref")
}

func TestReadProcessedObjectRejectsUnsupportedStores(t *testing.T) {
	t.Setenv(contentPipelineMediaObjectStoreEnv, "s3")

	_, err := ReadProcessedObject(context.Background(), defaultProcessedMediaObjectRefPrefix+"processed/ab/abcdef.png")

	require.Error(t, err)
	require.Contains(t, err.Error(), contentPipelineMediaObjectStoreEnv)
}

func TestReadProcessedObjectUsesConfiguredObjectRefPrefix(t *testing.T) {
	root := t.TempDir()
	key := "processed/ab/abcdef.png"
	path := filepath.Join(root, filepath.FromSlash(key))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte("image-bytes"), 0o600))
	t.Setenv(contentPipelineMediaObjectStoreEnv, "filesystem")
	t.Setenv(contentPipelineMediaObjectRootEnv, root)
	t.Setenv(contentPipelineMediaObjectRefPrefixEnv, "mpp://custom/media")

	data, err := ReadProcessedObject(context.Background(), "mpp://custom/media/"+key)

	require.NoError(t, err)
	require.Equal(t, []byte("image-bytes"), data)
}

func TestProcessedObjectKeyRejectsPathTraversal(t *testing.T) {
	_, err := processedObjectKey(defaultProcessedMediaObjectRefPrefix + "../secret.png")

	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid processed media object ref")
}

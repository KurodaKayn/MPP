package media

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
	objectstorager2 "github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage/r2"
)

const (
	defaultProcessedMediaObjectRefPrefix = "mpp://content-pipeline/media/"

	contentPipelineMediaObjectStoreEnv           = "CONTENT_PIPELINE_MEDIA_OBJECT_STORE"
	contentPipelineMediaObjectRootEnv            = "CONTENT_PIPELINE_MEDIA_OBJECT_ROOT"
	contentPipelineMediaObjectRefPrefixEnv       = "CONTENT_PIPELINE_MEDIA_OBJECT_REF_PREFIX"
	contentPipelineMediaObjectBucketEnv          = "CONTENT_PIPELINE_MEDIA_OBJECT_BUCKET"
	contentPipelineMediaObjectEndpointEnv        = "CONTENT_PIPELINE_MEDIA_OBJECT_ENDPOINT"
	contentPipelineMediaObjectRegionEnv          = "CONTENT_PIPELINE_MEDIA_OBJECT_REGION"
	contentPipelineMediaObjectAccessKeyIDEnv     = "CONTENT_PIPELINE_MEDIA_OBJECT_ACCESS_KEY_ID"
	contentPipelineMediaObjectSecretAccessKeyEnv = "CONTENT_PIPELINE_MEDIA_OBJECT_SECRET_ACCESS_KEY"

	r2AccountIDEnv       = "R2_ACCOUNT_ID"
	r2AccessKeyIDEnv     = "R2_ACCESS_KEY_ID"
	r2SecretAccessKeyEnv = "R2_SECRET_ACCESS_KEY"
	r2BucketEnv          = "R2_BUCKET"
	r2EndpointEnv        = "R2_ENDPOINT"
	r2RegionEnv          = "R2_REGION"

	defaultProcessedMediaRegion = "auto"
	defaultProcessedMediaTTL    = 5 * time.Minute
	r2EndpointTemplate          = "https://%s.r2.cloudflarestorage.com"
)

// ReadProcessedObject reads a content-pipeline processed object ref into memory
// for platform APIs that still require a multipart byte upload.
func ReadProcessedObject(ctx context.Context, objectRef string) ([]byte, error) {
	body, _, err := openProcessedObject(ctx, objectRef)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()

	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read processed media object: %w", err)
	}
	return data, nil
}

// MaterializeProcessedObject writes a processed object ref to a temporary file
// for browser automation APIs that require a local upload path.
func MaterializeProcessedObject(ctx context.Context, objectRef string, pattern string) (string, func(), error) {
	body, _, err := openProcessedObject(ctx, objectRef)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = body.Close() }()

	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, fmt.Errorf("create processed media temp file: %w", err)
	}
	cleanup := func() { _ = os.Remove(file.Name()) }

	if _, err := io.Copy(file, body); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, fmt.Errorf("write processed media temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close processed media temp file: %w", err)
	}
	return file.Name(), cleanup, nil
}

func openProcessedObject(ctx context.Context, objectRef string) (io.ReadCloser, objectstorage.ObjectInfo, error) {
	key, err := processedObjectKey(objectRef)
	if err != nil {
		return nil, objectstorage.ObjectInfo{}, err
	}

	switch strings.ToLower(envString(contentPipelineMediaObjectStoreEnv)) {
	case "filesystem":
		return openFilesystemProcessedObject(ctx, key)
	case "r2", "s3":
		client, err := processedObjectStorageClient()
		if err != nil {
			return nil, objectstorage.ObjectInfo{}, err
		}
		return client.GetObject(ctx, key)
	default:
		return nil, objectstorage.ObjectInfo{}, fmt.Errorf("%s must be configured to read processed media object refs", contentPipelineMediaObjectStoreEnv)
	}
}

func processedObjectKey(objectRef string) (string, error) {
	objectRef = strings.TrimSpace(objectRef)
	prefix := processedMediaObjectRefPrefix()
	if !strings.HasPrefix(objectRef, prefix) {
		return "", fmt.Errorf("unsupported processed media object ref")
	}
	key := strings.TrimPrefix(objectRef, prefix)
	if key == "" || strings.HasPrefix(key, "/") || strings.ContainsRune(key, '\x00') {
		return "", fmt.Errorf("invalid processed media object ref")
	}
	for segment := range strings.SplitSeq(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("invalid processed media object ref")
		}
	}
	return key, nil
}

func processedMediaObjectRefPrefix() string {
	prefix := envString(contentPipelineMediaObjectRefPrefixEnv)
	if prefix == "" {
		prefix = defaultProcessedMediaObjectRefPrefix
	}
	if strings.HasSuffix(prefix, "/") {
		return prefix
	}
	return prefix + "/"
}

func openFilesystemProcessedObject(ctx context.Context, key string) (io.ReadCloser, objectstorage.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, objectstorage.ObjectInfo{}, err
	}
	root := envString(contentPipelineMediaObjectRootEnv)
	if root == "" {
		return nil, objectstorage.ObjectInfo{}, fmt.Errorf("%s is required for filesystem processed media objects", contentPipelineMediaObjectRootEnv)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, objectstorage.ObjectInfo{}, fmt.Errorf("resolve processed media root: %w", err)
	}
	fullPath := filepath.Join(rootAbs, filepath.FromSlash(key))
	rel, err := filepath.Rel(rootAbs, fullPath)
	if err != nil {
		return nil, objectstorage.ObjectInfo{}, fmt.Errorf("resolve processed media object path: %w", err)
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return nil, objectstorage.ObjectInfo{}, fmt.Errorf("invalid processed media object path")
	}

	file, err := os.Open(fullPath) //nolint:gosec // Path is constrained to the configured processed-media root.
	if err != nil {
		return nil, objectstorage.ObjectInfo{}, fmt.Errorf("open processed media object: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, objectstorage.ObjectInfo{}, fmt.Errorf("stat processed media object: %w", err)
	}
	return file, objectstorage.ObjectInfo{
		Key:          key,
		Size:         info.Size(),
		LastModified: info.ModTime(),
	}, nil
}

func processedObjectStorageClient() (objectstorage.Client, error) {
	config, err := processedObjectStorageConfig()
	if err != nil {
		return nil, err
	}
	return objectstorager2.NewClient(config)
}

func processedObjectStorageConfig() (objectstorage.Config, error) {
	bucket := firstEnv(contentPipelineMediaObjectBucketEnv, r2BucketEnv)
	endpoint := strings.TrimRight(firstEnv(contentPipelineMediaObjectEndpointEnv, r2EndpointEnv), "/")
	if endpoint == "" {
		if accountID := envString(r2AccountIDEnv); accountID != "" {
			endpoint = fmt.Sprintf(r2EndpointTemplate, accountID)
		}
	}
	region := firstEnv(contentPipelineMediaObjectRegionEnv, r2RegionEnv)
	if region == "" {
		region = defaultProcessedMediaRegion
	}

	config := objectstorage.Config{
		Enabled:         true,
		Provider:        objectstorage.ProviderR2,
		Endpoint:        endpoint,
		Region:          region,
		Bucket:          bucket,
		AccessKeyID:     firstEnv(contentPipelineMediaObjectAccessKeyIDEnv, r2AccessKeyIDEnv),
		SecretAccessKey: firstEnv(contentPipelineMediaObjectSecretAccessKeyEnv, r2SecretAccessKeyEnv),
		DownloadURLTTL:  defaultProcessedMediaTTL,
	}
	if strings.TrimSpace(config.Bucket) == "" {
		return objectstorage.Config{}, fmt.Errorf("%s or %s is required for processed media objects", contentPipelineMediaObjectBucketEnv, r2BucketEnv)
	}
	return config, nil
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := envString(name); value != "" {
			return value
		}
	}
	return ""
}

func envString(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

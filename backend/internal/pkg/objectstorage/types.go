package objectstorage

import (
	"context"
	"errors"
	"io"
	"time"
)

const (
	// ProviderDisabled disables object storage integration.
	ProviderDisabled = "disabled"
	// ProviderR2 enables the Cloudflare R2 S3-compatible object storage client.
	ProviderR2 = "r2"
)

// ErrObjectNotFound is returned when the requested object does not exist.
var ErrObjectNotFound = errors.New("object not found")

// Client is the storage boundary used by media upload and publishing flows.
type Client interface {
	PresignPutObject(ctx context.Context, input PutObjectInput) (PresignedURL, error)
	PresignGetObject(ctx context.Context, input GetObjectInput) (PresignedURL, error)
	HeadObject(ctx context.Context, key string) (ObjectInfo, error)
	GetObject(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)
	DeleteObject(ctx context.Context, key string) error
}

// Config contains object storage provider settings.
type Config struct {
	Enabled         bool
	Provider        string
	AccountID       string
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UploadURLTTL    time.Duration
	DownloadURLTTL  time.Duration
}

// PutObjectInput describes a signed PUT request.
type PutObjectInput struct {
	Bucket      string
	Key         string
	ContentType string
	Expires     time.Duration
}

// GetObjectInput describes a signed GET request.
type GetObjectInput struct {
	Bucket  string
	Key     string
	Expires time.Duration
}

// PresignedURL is a temporary URL and the headers required to use it.
type PresignedURL struct {
	URL     string
	Headers map[string]string
	Expires time.Duration
}

// ObjectInfo describes object metadata returned by storage providers.
type ObjectInfo struct {
	Key          string
	ContentType  string
	Size         int64
	ETag         string
	LastModified time.Time
}

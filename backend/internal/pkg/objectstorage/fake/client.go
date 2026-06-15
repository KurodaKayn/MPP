package fake

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
)

// Client is an in-memory object storage client for tests.
type Client struct {
	mu                sync.RWMutex
	objects           map[string]storedObject
	presignGetObjects int
}

type storedObject struct {
	body []byte
	info objectstorage.ObjectInfo
}

// NewClient creates an empty fake object storage client.
func NewClient() *Client {
	return &Client{objects: map[string]storedObject{}}
}

// PutObject stores an object in memory.
func (c *Client) PutObject(ctx context.Context, input objectstorage.UploadObjectInput) (objectstorage.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return objectstorage.ObjectInfo{}, err
	}
	if input.Body == nil {
		return objectstorage.ObjectInfo{}, errors.New("object body is required")
	}
	body, err := io.ReadAll(input.Body)
	if err != nil {
		return objectstorage.ObjectInfo{}, err
	}

	info := objectstorage.ObjectInfo{
		Key:          input.Key,
		ContentType:  input.ContentType,
		Size:         int64(len(body)),
		LastModified: time.Now().UTC(),
	}
	c.StoreObject(input.Key, body, info)
	return info, nil
}

// PresignPutObject returns a deterministic fake PUT URL.
func (c *Client) PresignPutObject(ctx context.Context, input objectstorage.PutObjectInput) (objectstorage.PresignedURL, error) {
	if err := ctx.Err(); err != nil {
		return objectstorage.PresignedURL{}, err
	}
	headers := map[string]string{}
	if input.ContentType != "" {
		headers["Content-Type"] = input.ContentType
	}
	return objectstorage.PresignedURL{
		URL:     "fake://put/" + input.Bucket + "/" + input.Key,
		Headers: headers,
		Expires: input.Expires,
	}, nil
}

// PresignGetObject returns a deterministic fake GET URL.
func (c *Client) PresignGetObject(ctx context.Context, input objectstorage.GetObjectInput) (objectstorage.PresignedURL, error) {
	if err := ctx.Err(); err != nil {
		return objectstorage.PresignedURL{}, err
	}
	c.mu.Lock()
	c.presignGetObjects++
	c.mu.Unlock()
	return objectstorage.PresignedURL{
		URL:     "fake://get/" + input.Bucket + "/" + input.Key,
		Headers: map[string]string{},
		Expires: input.Expires,
	}, nil
}

func (c *Client) PresignGetObjectCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.presignGetObjects
}

// StoreObject inserts or replaces an object in the fake client.
func (c *Client) StoreObject(key string, body []byte, info objectstorage.ObjectInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if info.Key == "" {
		info.Key = key
	}
	c.objects[key] = storedObject{
		body: append([]byte(nil), body...),
		info: info,
	}
}

// HeadObject returns metadata for a stored fake object.
func (c *Client) HeadObject(ctx context.Context, key string) (objectstorage.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return objectstorage.ObjectInfo{}, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	object, ok := c.objects[key]
	if !ok {
		return objectstorage.ObjectInfo{}, objectstorage.ErrObjectNotFound
	}
	return object.info, nil
}

// GetObject returns a readable body and metadata for a stored fake object.
func (c *Client) GetObject(ctx context.Context, key string) (io.ReadCloser, objectstorage.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, objectstorage.ObjectInfo{}, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	object, ok := c.objects[key]
	if !ok {
		return nil, objectstorage.ObjectInfo{}, objectstorage.ErrObjectNotFound
	}
	return io.NopCloser(bytes.NewReader(object.body)), object.info, nil
}

// CopyObject copies a stored fake object to another key.
func (c *Client) CopyObject(ctx context.Context, input objectstorage.CopyObjectInput) (objectstorage.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return objectstorage.ObjectInfo{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	object, ok := c.objects[input.SourceKey]
	if !ok {
		return objectstorage.ObjectInfo{}, objectstorage.ErrObjectNotFound
	}
	info := object.info
	info.Key = input.DestinationKey
	c.objects[input.DestinationKey] = storedObject{
		body: append([]byte(nil), object.body...),
		info: info,
	}
	return info, nil
}

// DeleteObject removes a stored fake object.
func (c *Client) DeleteObject(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.objects, key)
	return nil
}

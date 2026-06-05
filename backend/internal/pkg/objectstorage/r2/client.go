package r2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/kurodakayn/mpp-backend/internal/pkg/objectstorage"
)

// Client stores and signs objects through Cloudflare R2's S3-compatible API.
type Client struct {
	bucket  string
	s3      *s3.Client
	presign *s3.PresignClient
}

// NewClient creates an R2-backed object storage client.
func NewClient(config objectstorage.Config) (*Client, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	awsConfig := aws.Config{
		Region:      config.Region,
		Credentials: credentials.NewStaticCredentialsProvider(config.AccessKeyID, config.SecretAccessKey, ""),
		EndpointResolverWithOptions: aws.EndpointResolverWithOptionsFunc(func(service string, region string, options ...interface{}) (aws.Endpoint, error) {
			if service != s3.ServiceID {
				return aws.Endpoint{}, &aws.EndpointNotFoundError{}
			}
			return aws.Endpoint{
				URL:           strings.TrimRight(config.Endpoint, "/"),
				SigningRegion: config.Region,
			}, nil
		}),
	}
	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.UsePathStyle = true
	})
	return &Client{
		bucket:  config.Bucket,
		s3:      client,
		presign: s3.NewPresignClient(client),
	}, nil
}

// PresignPutObject creates a temporary PUT URL for an R2 object.
func (c *Client) PresignPutObject(ctx context.Context, input objectstorage.PutObjectInput) (objectstorage.PresignedURL, error) {
	bucket := c.bucketFor(input.Bucket)
	ttl := expires(input.Expires)
	request, err := c.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(input.Key),
		ContentType: contentType(input.ContentType),
	}, func(options *s3.PresignOptions) {
		options.Expires = ttl
	})
	if err != nil {
		return objectstorage.PresignedURL{}, fmt.Errorf("presign R2 put object: %w", err)
	}
	return objectstorage.PresignedURL{
		URL:     request.URL,
		Headers: signedHeaders(request.SignedHeader, input.ContentType),
		Expires: ttl,
	}, nil
}

// PresignGetObject creates a temporary GET URL for an R2 object.
func (c *Client) PresignGetObject(ctx context.Context, input objectstorage.GetObjectInput) (objectstorage.PresignedURL, error) {
	bucket := c.bucketFor(input.Bucket)
	ttl := expires(input.Expires)
	request, err := c.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(input.Key),
	}, func(options *s3.PresignOptions) {
		options.Expires = ttl
	})
	if err != nil {
		return objectstorage.PresignedURL{}, fmt.Errorf("presign R2 get object: %w", err)
	}
	return objectstorage.PresignedURL{
		URL:     request.URL,
		Headers: signedHeaders(request.SignedHeader, ""),
		Expires: ttl,
	}, nil
}

// HeadObject returns R2 object metadata.
func (c *Client) HeadObject(ctx context.Context, key string) (objectstorage.ObjectInfo, error) {
	output, err := c.s3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return objectstorage.ObjectInfo{}, mapObjectError(err)
	}
	return objectstorage.ObjectInfo{
		Key:          key,
		ContentType:  aws.ToString(output.ContentType),
		Size:         aws.ToInt64(output.ContentLength),
		ETag:         trimETag(aws.ToString(output.ETag)),
		LastModified: aws.ToTime(output.LastModified),
	}, nil
}

// GetObject returns an R2 object body and metadata.
func (c *Client) GetObject(ctx context.Context, key string) (io.ReadCloser, objectstorage.ObjectInfo, error) {
	output, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, objectstorage.ObjectInfo{}, mapObjectError(err)
	}
	return output.Body, objectstorage.ObjectInfo{
		Key:          key,
		ContentType:  aws.ToString(output.ContentType),
		Size:         aws.ToInt64(output.ContentLength),
		ETag:         trimETag(aws.ToString(output.ETag)),
		LastModified: aws.ToTime(output.LastModified),
	}, nil
}

// CopyObject copies an R2 object to another key and returns destination metadata.
func (c *Client) CopyObject(ctx context.Context, input objectstorage.CopyObjectInput) (objectstorage.ObjectInfo, error) {
	sourceBucket := c.bucketFor(input.SourceBucket)
	destinationBucket := c.bucketFor(input.DestinationBucket)
	source := url.PathEscape(sourceBucket + "/" + input.SourceKey)
	_, err := c.s3.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(destinationBucket),
		Key:        aws.String(input.DestinationKey),
		CopySource: aws.String(source),
	})
	if err != nil {
		return objectstorage.ObjectInfo{}, mapObjectError(err)
	}
	return c.HeadObject(ctx, input.DestinationKey)
}

// DeleteObject removes an R2 object.
func (c *Client) DeleteObject(ctx context.Context, key string) error {
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return mapObjectError(err)
	}
	return nil
}

func validateConfig(config objectstorage.Config) error {
	if !config.Enabled || config.Provider != objectstorage.ProviderR2 {
		return fmt.Errorf("R2 object storage is not enabled")
	}
	required := map[string]string{
		"endpoint":          config.Endpoint,
		"region":            config.Region,
		"bucket":            config.Bucket,
		"access key ID":     config.AccessKeyID,
		"secret access key": config.SecretAccessKey,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("R2 %s is required", name)
		}
	}
	return nil
}

func (c *Client) bucketFor(bucket string) string {
	if strings.TrimSpace(bucket) != "" {
		return strings.TrimSpace(bucket)
	}
	return c.bucket
}

func contentType(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return aws.String(strings.TrimSpace(value))
}

func expires(value time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return 15 * time.Minute
}

func signedHeaders(headers http.Header, contentType string) map[string]string {
	result := map[string]string{}
	for key, values := range headers {
		if strings.EqualFold(key, "host") {
			continue
		}
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	if strings.TrimSpace(contentType) != "" {
		result["Content-Type"] = strings.TrimSpace(contentType)
	}
	return result
}

func mapObjectError(err error) error {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return objectstorage.ErrObjectNotFound
		}
	}
	return err
}

func trimETag(value string) string {
	return strings.Trim(value, `"`)
}

package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/contracts/contentpipelinepb"
	"github.com/kurodakayn/mpp-backend/internal/pkg/envutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	contentPipelineMediaEnabledEnv = "CONTENT_PIPELINE_MEDIA_ENABLED"
	contentPipelineAddrEnv         = "CONTENT_PIPELINE_GRPC_ADDR"
	defaultContentPipelineAddr     = "127.0.0.1:50051"
	contentPipelineRequestTimeout  = 20 * time.Second
)

var errContentPipelineContract = errors.New("content pipeline contract error")

type contentPipelineMediaClientFactory func(context.Context) (contentpipelinepb.MediaAssetProcessorClient, io.Closer, error)

var newContentPipelineMediaClient contentPipelineMediaClientFactory = dialContentPipelineMediaClient

func contentPipelineMediaEnabled() bool {
	return envutil.Bool(contentPipelineMediaEnabledEnv, false)
}

func processWithContentPipeline(sourceURL string, platform string, usage string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), contentPipelineRequestTimeout)
	defer cancel()

	client, closer, err := newContentPipelineMediaClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect content pipeline: %w", err)
	}
	if closer != nil {
		defer closer.Close()
	}

	response, err := client.ProcessAsset(ctx, &contentpipelinepb.ProcessAssetRequest{
		RequestId: uuid.NewString(),
		Platform:  strings.TrimSpace(platform),
		Usage:     strings.TrimSpace(usage),
		Source:    mediaSourceFromURL(sourceURL),
		Constraints: &contentpipelinepb.MediaConstraints{
			MaxBytes: MaxWechatSize,
		},
	})
	if err != nil {
		return nil, err
	}

	asset := response.GetAsset()
	if asset == nil {
		return nil, fmt.Errorf("%w: missing processed asset", errContentPipelineContract)
	}
	inlineBytes := asset.GetInlineBytes()
	if inlineBytes == nil {
		return nil, fmt.Errorf("%w: processed asset did not include inline bytes", errContentPipelineContract)
	}
	return inlineBytes, nil
}

func dialContentPipelineMediaClient(_ context.Context) (contentpipelinepb.MediaAssetProcessorClient, io.Closer, error) {
	conn, err := grpc.NewClient(contentPipelineAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return contentpipelinepb.NewMediaAssetProcessorClient(conn), conn, nil
}

func contentPipelineAddr() string {
	if value := strings.TrimSpace(os.Getenv(contentPipelineAddrEnv)); value != "" {
		return value
	}
	return defaultContentPipelineAddr
}

func mediaSourceFromURL(sourceURL string) *contentpipelinepb.MediaSource {
	sourceURL = strings.TrimSpace(sourceURL)
	if strings.HasPrefix(strings.ToLower(sourceURL), "data:") {
		return &contentpipelinepb.MediaSource{
			Value: &contentpipelinepb.MediaSource_DataUrl{DataUrl: sourceURL},
		}
	}
	return &contentpipelinepb.MediaSource{
		Value: &contentpipelinepb.MediaSource_Url{Url: sourceURL},
	}
}

func shouldFallbackContentPipelineError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errContentPipelineContract) {
		return false
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Unimplemented, codes.Unknown:
		return true
	default:
		return false
	}
}

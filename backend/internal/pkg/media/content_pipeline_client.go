package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kurodakayn/mpp-backend/internal/contracts/contentpipelinepb"
	"github.com/kurodakayn/mpp-backend/internal/pkg/contentpipeline"
	"github.com/kurodakayn/mpp-backend/internal/pkg/envutil"
)

const (
	contentPipelineMediaEnabledEnv = "CONTENT_PIPELINE_MEDIA_ENABLED"
	contentPipelineHostEnv         = contentpipeline.HostEnv
	contentPipelinePortEnv         = contentpipeline.PortEnv
	contentPipelineRequestTimeout  = 20 * time.Second
	mediaObjectRefPrefix           = "mpp://media/"
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
		defer func() { _ = closer.Close() }()
	}

	response, err := client.ProcessAsset(ctx, &contentpipelinepb.ProcessAssetRequest{
		RequestId:   uuid.NewString(),
		Platform:    strings.TrimSpace(platform),
		Usage:       strings.TrimSpace(usage),
		Source:      mediaSourceFromURL(sourceURL),
		Constraints: mediaConstraintsForPlatform(platform),
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

func mediaConstraintsForPlatform(platform string) *contentpipelinepb.MediaConstraints {
	if strings.EqualFold(strings.TrimSpace(platform), "wechat") {
		return &contentpipelinepb.MediaConstraints{
			MaxBytes: MaxWechatSize,
		}
	}
	return nil
}

func dialContentPipelineMediaClient(_ context.Context) (contentpipelinepb.MediaAssetProcessorClient, io.Closer, error) {
	conn, err := contentpipeline.Dial()
	if err != nil {
		return nil, nil, err
	}
	return contentpipelinepb.NewMediaAssetProcessorClient(conn), conn, nil
}

func contentPipelineAddr() string {
	return contentpipeline.Addr()
}

func mediaSourceFromURL(sourceURL string) *contentpipelinepb.MediaSource {
	sourceURL = strings.TrimSpace(sourceURL)
	if strings.HasPrefix(strings.ToLower(sourceURL), "data:") {
		return &contentpipelinepb.MediaSource{
			Value: &contentpipelinepb.MediaSource_DataUrl{DataUrl: sourceURL},
		}
	}
	if isMediaObjectRef(sourceURL) {
		return &contentpipelinepb.MediaSource{
			Value: &contentpipelinepb.MediaSource_ObjectRef{ObjectRef: sourceURL},
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

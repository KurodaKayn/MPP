package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kurodakayn/mpp-backend/internal/contracts/contentpipelinepb"
	"github.com/kurodakayn/mpp-backend/internal/pkg/contentpipeline"
)

const (
	contentPipelineHostEnv        = contentpipeline.HostEnv
	contentPipelinePortEnv        = contentpipeline.PortEnv
	contentPipelineRequestTimeout = 20 * time.Second
	mediaObjectRefPrefix          = "mpp://media/"
)

var errContentPipelineContract = errors.New("content pipeline contract error")

type contentPipelineMediaClientFactory func(context.Context) (contentpipelinepb.MediaAssetProcessorClient, io.Closer, error)

var newContentPipelineMediaClient contentPipelineMediaClientFactory = dialContentPipelineMediaClient

func processWithContentPipeline(sourceURL string, platform string, usage string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), contentPipelineRequestTimeout)
	defer cancel()

	client, closer, err := newContentPipelineMediaClient(ctx)
	if err != nil {
		return "", fmt.Errorf("connect content pipeline: %w", err)
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}

	response, err := client.ProcessAsset(ctx, &contentpipelinepb.ProcessAssetRequest{
		RequestId: uuid.NewString(),
		Platform:  strings.TrimSpace(platform),
		Usage:     strings.TrimSpace(usage),
		Source:    mediaSourceFromURL(sourceURL),
	})
	if err != nil {
		return "", err
	}

	asset := response.GetAsset()
	if asset == nil {
		return "", fmt.Errorf("%w: missing processed asset", errContentPipelineContract)
	}
	objectRef := strings.TrimSpace(asset.GetObjectRef())
	if objectRef == "" {
		return "", fmt.Errorf("%w: processed asset did not include object ref", errContentPipelineContract)
	}
	return objectRef, nil
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

package media

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kurodakayn/mpp-backend/internal/contracts/contentpipelinepb"
)

type fakeContentPipelineMediaClient struct {
	request  *contentpipelinepb.ProcessAssetRequest
	response *contentpipelinepb.ProcessAssetResponse
	err      error
}

func (f *fakeContentPipelineMediaClient) ProcessAsset(_ context.Context, request *contentpipelinepb.ProcessAssetRequest, _ ...grpc.CallOption) (*contentpipelinepb.ProcessAssetResponse, error) {
	f.request = request
	return f.response, f.err
}

type noopCloser struct{}

func (noopCloser) Close() error {
	return nil
}

func TestDownloadAndProcessDelegatesToContentPipeline(t *testing.T) {
	fakeClient := &fakeContentPipelineMediaClient{
		response: &contentpipelinepb.ProcessAssetResponse{
			Asset: &contentpipelinepb.ProcessedAsset{
				ObjectRef: "mpp://content-pipeline/media/processed/aa/asset.jpg",
				MimeType:  "image/jpeg",
			},
			Status: "processed",
		},
	}
	withContentPipelineMediaClientFactory(t, func(context.Context) (contentpipelinepb.MediaAssetProcessorClient, io.Closer, error) {
		return fakeClient, noopCloser{}, nil
	})

	objectRef, err := DownloadAndProcessForPlatform("data:image/png;base64,aGVsbG8=", "wechat", "cover")

	require.NoError(t, err)
	require.Equal(t, "mpp://content-pipeline/media/processed/aa/asset.jpg", objectRef)
	require.Equal(t, "wechat", fakeClient.request.GetPlatform())
	require.Equal(t, "cover", fakeClient.request.GetUsage())
	require.Nil(t, fakeClient.request.GetConstraints())
	require.Equal(t, "data:image/png;base64,aGVsbG8=", fakeClient.request.GetSource().GetDataUrl())
}

func TestDownloadAndProcessSendsMediaObjectRefsToContentPipeline(t *testing.T) {
	fakeClient := &fakeContentPipelineMediaClient{
		response: &contentpipelinepb.ProcessAssetResponse{
			Asset: &contentpipelinepb.ProcessedAsset{
				ObjectRef: "mpp://content-pipeline/media/processed/bb/asset.jpg",
				MimeType:  "image/jpeg",
			},
			Status: "processed",
		},
	}
	withContentPipelineMediaClientFactory(t, func(context.Context) (contentpipelinepb.MediaAssetProcessorClient, io.Closer, error) {
		return fakeClient, noopCloser{}, nil
	})

	source := "mpp://media/11111111-1111-4111-8111-111111111111"
	objectRef, err := DownloadAndProcessForPlatform(source, "wechat", "cover")

	require.NoError(t, err)
	require.Equal(t, "mpp://content-pipeline/media/processed/bb/asset.jpg", objectRef)
	require.Equal(t, source, fakeClient.request.GetSource().GetObjectRef())
	require.Empty(t, fakeClient.request.GetSource().GetUrl())
}

func TestDownloadAndProcessUsesContentPipelineDefaultsForNonWechatPlatforms(t *testing.T) {
	fakeClient := &fakeContentPipelineMediaClient{
		response: &contentpipelinepb.ProcessAssetResponse{
			Asset: &contentpipelinepb.ProcessedAsset{
				ObjectRef: "mpp://content-pipeline/media/processed/cc/asset.jpg",
				MimeType:  "image/jpeg",
			},
			Status: "processed",
		},
	}
	withContentPipelineMediaClientFactory(t, func(context.Context) (contentpipelinepb.MediaAssetProcessorClient, io.Closer, error) {
		return fakeClient, noopCloser{}, nil
	})

	objectRef, err := DownloadAndProcessForPlatform("data:image/png;base64,aGVsbG8=", "douyin", "cover")

	require.NoError(t, err)
	require.Equal(t, "mpp://content-pipeline/media/processed/cc/asset.jpg", objectRef)
	require.Equal(t, "douyin", fakeClient.request.GetPlatform())
	require.Nil(t, fakeClient.request.GetConstraints())
}

func TestDownloadAndProcessPropagatesTransientContentPipelineErrors(t *testing.T) {
	fakeClient := &fakeContentPipelineMediaClient{
		err: status.Error(codes.Unavailable, "content pipeline unavailable"),
	}
	withContentPipelineMediaClientFactory(t, func(context.Context) (contentpipelinepb.MediaAssetProcessorClient, io.Closer, error) {
		return fakeClient, noopCloser{}, nil
	})

	objectRef, err := DownloadAndProcess("data:image/png;base64,aGVsbG8=")

	require.Error(t, err)
	require.Empty(t, objectRef)
	require.Contains(t, err.Error(), "content pipeline unavailable")
}

func TestDownloadAndProcessPropagatesObjectRefContentPipelineErrors(t *testing.T) {
	fakeClient := &fakeContentPipelineMediaClient{
		err: status.Error(codes.Unavailable, "content pipeline unavailable"),
	}
	withContentPipelineMediaClientFactory(t, func(context.Context) (contentpipelinepb.MediaAssetProcessorClient, io.Closer, error) {
		return fakeClient, noopCloser{}, nil
	})

	objectRef, err := DownloadAndProcess("mpp://media/11111111-1111-4111-8111-111111111111")

	require.Error(t, err)
	require.Empty(t, objectRef)
	require.Contains(t, err.Error(), "content pipeline unavailable")
}

func TestDownloadAndProcessPropagatesContentPipelineValidationErrors(t *testing.T) {
	fakeClient := &fakeContentPipelineMediaClient{
		err: status.Error(codes.InvalidArgument, "unsafe media URL"),
	}
	withContentPipelineMediaClientFactory(t, func(context.Context) (contentpipelinepb.MediaAssetProcessorClient, io.Closer, error) {
		return fakeClient, noopCloser{}, nil
	})

	objectRef, err := DownloadAndProcess("data:image/png;base64,aGVsbG8=")

	require.Error(t, err)
	require.Empty(t, objectRef)
}

func TestContentPipelineAddrUsesConfiguredHostAndPort(t *testing.T) {
	t.Setenv(contentPipelineHostEnv, "pipeline")
	t.Setenv(contentPipelinePortEnv, "50052")

	require.Equal(t, "pipeline:50052", contentPipelineAddr())
}

func TestContentPipelineAddrFallsBackToServiceNameAndDefaultPort(t *testing.T) {
	t.Setenv(contentPipelineHostEnv, " ")
	t.Setenv(contentPipelinePortEnv, " ")

	require.Equal(t, "content-pipeline-service:50051", contentPipelineAddr())
}

func TestContentPipelineAddrSupportsIPv6Hosts(t *testing.T) {
	t.Setenv(contentPipelineHostEnv, "::1")
	t.Setenv(contentPipelinePortEnv, "50051")

	require.Equal(t, "[::1]:50051", contentPipelineAddr())
}

func withContentPipelineMediaClientFactory(t *testing.T, factory contentPipelineMediaClientFactory) {
	t.Helper()
	previous := newContentPipelineMediaClient
	newContentPipelineMediaClient = factory
	t.Cleanup(func() {
		newContentPipelineMediaClient = previous
	})
}

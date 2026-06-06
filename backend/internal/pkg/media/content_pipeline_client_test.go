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

func TestDownloadAndProcessUsesContentPipelineWhenEnabled(t *testing.T) {
	t.Setenv(contentPipelineMediaEnabledEnv, "true")
	fakeClient := &fakeContentPipelineMediaClient{
		response: &contentpipelinepb.ProcessAssetResponse{
			Asset: &contentpipelinepb.ProcessedAsset{
				Content: &contentpipelinepb.ProcessedAsset_InlineBytes{
					InlineBytes: []byte("processed-by-rust"),
				},
				MimeType: "image/jpeg",
			},
			Status: "processed",
		},
	}
	withContentPipelineMediaClientFactory(t, func(context.Context) (contentpipelinepb.MediaAssetProcessorClient, io.Closer, error) {
		return fakeClient, noopCloser{}, nil
	})

	data, err := DownloadAndProcessForPlatform("data:image/png;base64,aGVsbG8=", "wechat", "cover")

	require.NoError(t, err)
	require.Equal(t, []byte("processed-by-rust"), data)
	require.Equal(t, "wechat", fakeClient.request.GetPlatform())
	require.Equal(t, "cover", fakeClient.request.GetUsage())
	require.Equal(t, uint64(MaxWechatSize), fakeClient.request.GetConstraints().GetMaxBytes())
	require.Equal(t, "data:image/png;base64,aGVsbG8=", fakeClient.request.GetSource().GetDataUrl())
}

func TestDownloadAndProcessSendsMediaObjectRefsToContentPipeline(t *testing.T) {
	t.Setenv(contentPipelineMediaEnabledEnv, "true")
	fakeClient := &fakeContentPipelineMediaClient{
		response: &contentpipelinepb.ProcessAssetResponse{
			Asset: &contentpipelinepb.ProcessedAsset{
				Content: &contentpipelinepb.ProcessedAsset_InlineBytes{
					InlineBytes: []byte("processed-object-ref"),
				},
				MimeType: "image/jpeg",
			},
			Status: "processed",
		},
	}
	withContentPipelineMediaClientFactory(t, func(context.Context) (contentpipelinepb.MediaAssetProcessorClient, io.Closer, error) {
		return fakeClient, noopCloser{}, nil
	})

	source := "mpp://media/11111111-1111-4111-8111-111111111111"
	data, err := DownloadAndProcessForPlatform(source, "wechat", "cover")

	require.NoError(t, err)
	require.Equal(t, []byte("processed-object-ref"), data)
	require.Equal(t, source, fakeClient.request.GetSource().GetObjectRef())
	require.Empty(t, fakeClient.request.GetSource().GetUrl())
}

func TestDownloadAndProcessUsesContentPipelineDefaultsForNonWechatPlatforms(t *testing.T) {
	t.Setenv(contentPipelineMediaEnabledEnv, "true")
	fakeClient := &fakeContentPipelineMediaClient{
		response: &contentpipelinepb.ProcessAssetResponse{
			Asset: &contentpipelinepb.ProcessedAsset{
				Content: &contentpipelinepb.ProcessedAsset_InlineBytes{
					InlineBytes: []byte("processed-by-rust"),
				},
				MimeType: "image/jpeg",
			},
			Status: "processed",
		},
	}
	withContentPipelineMediaClientFactory(t, func(context.Context) (contentpipelinepb.MediaAssetProcessorClient, io.Closer, error) {
		return fakeClient, noopCloser{}, nil
	})

	data, err := DownloadAndProcessForPlatform("data:image/png;base64,aGVsbG8=", "douyin", "cover")

	require.NoError(t, err)
	require.Equal(t, []byte("processed-by-rust"), data)
	require.Equal(t, "douyin", fakeClient.request.GetPlatform())
	require.Nil(t, fakeClient.request.GetConstraints())
}

func TestDownloadAndProcessFallsBackForTransientContentPipelineErrors(t *testing.T) {
	t.Setenv(contentPipelineMediaEnabledEnv, "true")
	fakeClient := &fakeContentPipelineMediaClient{
		err: status.Error(codes.Unavailable, "content pipeline unavailable"),
	}
	withContentPipelineMediaClientFactory(t, func(context.Context) (contentpipelinepb.MediaAssetProcessorClient, io.Closer, error) {
		return fakeClient, noopCloser{}, nil
	})

	data, err := DownloadAndProcess("data:image/png;base64,aGVsbG8=")

	require.NoError(t, err)
	require.Equal(t, []byte("hello"), data)
}

func TestDownloadAndProcessDoesNotFallbackForMediaObjectRefs(t *testing.T) {
	t.Setenv(contentPipelineMediaEnabledEnv, "true")
	fakeClient := &fakeContentPipelineMediaClient{
		err: status.Error(codes.Unavailable, "content pipeline unavailable"),
	}
	withContentPipelineMediaClientFactory(t, func(context.Context) (contentpipelinepb.MediaAssetProcessorClient, io.Closer, error) {
		return fakeClient, noopCloser{}, nil
	})

	data, err := DownloadAndProcess("mpp://media/11111111-1111-4111-8111-111111111111")

	require.Error(t, err)
	require.Nil(t, data)
	require.Contains(t, err.Error(), "content pipeline unavailable")
}

func TestDownloadAndProcessDoesNotFallbackForContentPipelineValidationErrors(t *testing.T) {
	t.Setenv(contentPipelineMediaEnabledEnv, "true")
	fakeClient := &fakeContentPipelineMediaClient{
		err: status.Error(codes.InvalidArgument, "unsafe media URL"),
	}
	withContentPipelineMediaClientFactory(t, func(context.Context) (contentpipelinepb.MediaAssetProcessorClient, io.Closer, error) {
		return fakeClient, noopCloser{}, nil
	})

	data, err := DownloadAndProcess("data:image/png;base64,aGVsbG8=")

	require.Error(t, err)
	require.Nil(t, data)
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

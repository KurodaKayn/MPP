package compiler

import (
	"context"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"gorm.io/datatypes"

	"github.com/kurodakayn/mpp-backend/internal/contracts/contentpipelinepb"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

type fakePlatformDraftCompilerClient struct {
	request  *contentpipelinepb.CompileDraftsRequest
	response *contentpipelinepb.CompileDraftsResponse
	err      error
}

func (f *fakePlatformDraftCompilerClient) CompileDrafts(ctx context.Context, request *contentpipelinepb.CompileDraftsRequest, opts ...grpc.CallOption) (*contentpipelinepb.CompileDraftsResponse, error) {
	f.request = request
	return f.response, f.err
}

type draftNoopCloser struct{}

func (draftNoopCloser) Close() error {
	return nil
}

func TestContentPipelineDraftCompilerBuildsCompileRequest(t *testing.T) {
	projectID := uuid.New()
	fakeClient := &fakePlatformDraftCompilerClient{
		response: &contentpipelinepb.CompileDraftsResponse{
			Drafts: []*contentpipelinepb.CompiledDraft{
				{
					Platform:           "zhihu",
					Status:             "compiled",
					AdaptedContentJson: `{"format":"markdown","markdown":"## Draft"}`,
				},
				{
					Platform:           "x",
					Status:             "compiled",
					AdaptedContentJson: `{"format":"text","text":"Post"}`,
				},
			},
		},
	}
	compiler := &contentPipelineDraftCompiler{
		newClient: func(context.Context) (contentpipelinepb.PlatformDraftCompilerClient, io.Closer, error) {
			return fakeClient, draftNoopCloser{}, nil
		},
	}

	drafts, err := compiler.CompileProjectDrafts(
		context.Background(),
		&models.Project{
			ID:            projectID,
			Title:         "Draft title",
			SourceContent: "<p>Hello</p>",
		},
		[]models.ProjectPlatformPublication{
			{Platform: "zhihu", Config: datatypes.JSON(`{"title":"Zhihu"}`)},
			{Platform: "x", Config: datatypes.JSON(`{"title":"X"}`)},
		},
		[]string{"zhihu", "x"},
	)

	require.NoError(t, err)
	require.JSONEq(t, `{"format":"markdown","markdown":"## Draft"}`, string(drafts["zhihu"]))
	require.JSONEq(t, `{"format":"text","text":"Post"}`, string(drafts["x"]))
	require.Equal(t, projectID.String(), fakeClient.request.GetProject().GetId())
	require.Equal(t, "Draft title", fakeClient.request.GetProject().GetTitle())
	require.Equal(t, "html", fakeClient.request.GetProject().GetSourceFormat())
	require.Len(t, fakeClient.request.GetTargets(), 2)
	require.Equal(t, "zhihu@v1", fakeClient.request.GetTargets()[0].GetProfile())
	require.JSONEq(t, `{"title":"Zhihu"}`, fakeClient.request.GetTargets()[0].GetConfigJson())
}

func TestContentPipelineDraftCompilerRejectsWrongPlatformFormat(t *testing.T) {
	fakeClient := &fakePlatformDraftCompilerClient{
		response: &contentpipelinepb.CompileDraftsResponse{
			Drafts: []*contentpipelinepb.CompiledDraft{
				{
					Platform:           "zhihu",
					Status:             "compiled",
					AdaptedContentJson: `{"format":"text","text":"Not markdown"}`,
				},
			},
		},
	}
	compiler := &contentPipelineDraftCompiler{
		newClient: func(context.Context) (contentpipelinepb.PlatformDraftCompilerClient, io.Closer, error) {
			return fakeClient, draftNoopCloser{}, nil
		},
	}

	drafts, err := compiler.CompileProjectDrafts(
		context.Background(),
		&models.Project{ID: uuid.New(), Title: "Draft title", SourceContent: "<p>Hello</p>"},
		[]models.ProjectPlatformPublication{{Platform: "zhihu"}},
		[]string{"zhihu"},
	)

	require.Error(t, err)
	require.Nil(t, drafts)
	require.Contains(t, err.Error(), `expected "markdown" draft format`)
}

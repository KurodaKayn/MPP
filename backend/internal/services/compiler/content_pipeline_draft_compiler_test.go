package compiler

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/datatypes"

	"github.com/kurodakayn/mpp-backend/internal/contracts/contentpipelinepb"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

type fakePlatformDraftCompilerClient struct {
	request  *contentpipelinepb.CompileDraftsRequest
	response *contentpipelinepb.CompileDraftsResponse
	err      error
}

func (f *fakePlatformDraftCompilerClient) CompileDrafts(_ context.Context, request *contentpipelinepb.CompileDraftsRequest, _ ...grpc.CallOption) (*contentpipelinepb.CompileDraftsResponse, error) {
	f.request = request
	return f.response, f.err
}

type draftNoopCloser struct{}

func (draftNoopCloser) Close() error {
	return nil
}

type fakeProjectDraftCompiler struct {
	drafts map[string][]byte
	err    error
	calls  int
}

func (f *fakeProjectDraftCompiler) CompileProjectDrafts(context.Context, *models.Project, []models.ProjectPlatformPublication, []string) (map[string][]byte, error) {
	f.calls++
	return f.drafts, f.err
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

func TestNewContentPipelineDraftCompilerUsesFallbackWhenDisabled(t *testing.T) {
	t.Setenv(contentPipelineDraftsEnabledEnv, "false")
	compiler := NewContentPipelineDraftCompiler()

	drafts, err := compiler.CompileProjectDrafts(
		context.Background(),
		&models.Project{ID: uuid.New(), Title: "Draft title", SourceContent: "<p>Hello fallback</p>"},
		nil,
		[]string{"wechat"},
	)

	require.NoError(t, err)
	require.JSONEq(t, `{"schema_version":1,"format":"html","html":"<p>Hello fallback</p>","summary":"Hello fallback"}`, string(drafts["wechat"]))
}

func TestFallbackingDraftCompilerFallsBackForTransientErrors(t *testing.T) {
	primary := &fakeProjectDraftCompiler{err: status.Error(codes.Unavailable, "content pipeline unavailable")}
	fallback := &fakeProjectDraftCompiler{drafts: map[string][]byte{
		"zhihu": []byte(`{"format":"markdown","markdown":"Fallback"}`),
	}}
	compiler := &fallbackingDraftCompiler{primary: primary, fallback: fallback}

	drafts, err := compiler.CompileProjectDrafts(
		context.Background(),
		&models.Project{ID: uuid.New(), Title: "Draft title", SourceContent: "<p>Hello</p>"},
		nil,
		[]string{"zhihu"},
	)

	require.NoError(t, err)
	require.Equal(t, 1, primary.calls)
	require.Equal(t, 1, fallback.calls)
	require.JSONEq(t, `{"format":"markdown","markdown":"Fallback"}`, string(drafts["zhihu"]))
}

func TestFallbackingDraftCompilerDoesNotFallbackForContractErrors(t *testing.T) {
	primary := &fakeProjectDraftCompiler{err: fmt.Errorf("%w: missing compiled draft", errContentPipelineDraftContract)}
	fallback := &fakeProjectDraftCompiler{drafts: map[string][]byte{
		"zhihu": []byte(`{"format":"markdown","markdown":"Fallback"}`),
	}}
	compiler := &fallbackingDraftCompiler{primary: primary, fallback: fallback}

	drafts, err := compiler.CompileProjectDrafts(
		context.Background(),
		&models.Project{ID: uuid.New(), Title: "Draft title", SourceContent: "<p>Hello</p>"},
		nil,
		[]string{"zhihu"},
	)

	require.ErrorIs(t, err, errContentPipelineDraftContract)
	require.Nil(t, drafts)
	require.Equal(t, 1, primary.calls)
	require.Zero(t, fallback.calls)
}

func TestFallbackDraftCompilerCompilesPlatformDrafts(t *testing.T) {
	compiler := NewFallbackDraftCompiler()

	drafts, err := compiler.CompileProjectDrafts(
		context.Background(),
		&models.Project{
			ID:    uuid.New(),
			Title: "Launch Notes",
			SourceContent: `
				<h2>Heading</h2>
				<p>Hello <strong>draft</strong> with <a href="https://example.com/release">link</a>.</p>
				<blockquote>Stable fallback</blockquote>
				<p><img src="https://example.com/cover.png" alt="Cover"></p>
			`,
		},
		nil,
		[]string{"wechat", "zhihu", "x", "douyin"},
	)

	require.NoError(t, err)
	require.JSONEq(t, `{"schema_version":1,"format":"html","html":"\n\t\t\t\t<h2>Heading</h2>\n\t\t\t\t<p>Hello <strong>draft</strong> with <a href=\"https://example.com/release\">link</a>.</p>\n\t\t\t\t<blockquote>Stable fallback</blockquote>\n\t\t\t\t<p><img src=\"https://example.com/cover.png\" alt=\"Cover\"></p>\n\t\t\t","summary":"Heading\nHello draft with link.\nStable fallback"}`, string(drafts["wechat"]))
	require.Contains(t, string(drafts["zhihu"]), `"format":"markdown"`)
	require.Contains(t, string(drafts["zhihu"]), `## Heading`)
	require.Contains(t, string(drafts["zhihu"]), `**draft**`)
	require.Contains(t, string(drafts["zhihu"]), `[link](https://example.com/release)`)
	require.Contains(t, string(drafts["zhihu"]), `![Cover](https://example.com/cover.png)`)
	require.Contains(t, string(drafts["x"]), `"format":"text"`)
	require.Contains(t, string(drafts["x"]), `Launch Notes`)
	require.Contains(t, string(drafts["x"]), `Hello draft with link.`)
	require.Contains(t, string(drafts["douyin"]), `"format":"text"`)
	require.Contains(t, string(drafts["douyin"]), `Stable fallback`)
}

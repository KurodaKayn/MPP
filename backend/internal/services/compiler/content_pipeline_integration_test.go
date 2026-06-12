//go:build contentpipeline_integration

package compiler

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

func TestContentPipelineIntegrationCompilesDrafts(t *testing.T) {
	draftCompiler := NewContentPipelineDraftCompiler()

	drafts, err := draftCompiler.CompileProjectDrafts(
		context.Background(),
		&models.Project{
			ID:            uuid.New(),
			Title:         "Integration Draft",
			SourceContent: `<h2>Integration</h2><p>Hello <strong>pipeline</strong>.</p>`,
		},
		[]models.ProjectPlatformPublication{
			{Platform: "wechat"},
			{Platform: "zhihu"},
			{Platform: "x"},
		},
		[]string{"wechat", "zhihu", "x"},
	)

	require.NoError(t, err)
	require.JSONEq(t, `{"schema_version":1,"format":"html","html":"<h2>Integration</h2><p>Hello <strong>pipeline</strong>.</p>","summary":"Integration\nHello pipeline."}`, string(drafts["wechat"]))
	require.Contains(t, string(drafts["zhihu"]), `"format":"markdown"`)
	require.Contains(t, string(drafts["zhihu"]), `## Integration`)
	require.Contains(t, string(drafts["x"]), `"format":"text"`)
	require.Contains(t, string(drafts["x"]), `Integration Draft`)
}

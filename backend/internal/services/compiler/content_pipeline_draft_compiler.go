package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kurodakayn/mpp-backend/internal/contracts/contentpipelinepb"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/pkg/contentpipeline"
	"github.com/kurodakayn/mpp-backend/internal/pkg/envutil"
)

const (
	contentPipelineDraftsEnabledEnv = "CONTENT_PIPELINE_DRAFTS_ENABLED"
	contentPipelineDraftTimeout     = 20 * time.Second
)

var errContentPipelineDraftContract = errors.New("content pipeline draft contract error")

type ProjectDraftCompiler interface {
	CompileProjectDrafts(ctx context.Context, project *models.Project, publications []models.ProjectPlatformPublication, platforms []string) (map[string][]byte, error)
}

type contentPipelineDraftCompilerClientFactory func(context.Context) (contentpipelinepb.PlatformDraftCompilerClient, io.Closer, error)

type contentPipelineDraftCompiler struct {
	newClient contentPipelineDraftCompilerClientFactory
}

func NewContentPipelineDraftCompiler() ProjectDraftCompiler {
	fallback := NewFallbackDraftCompiler()
	if !contentPipelineDraftsEnabled() {
		return fallback
	}
	return &fallbackingDraftCompiler{
		primary: &contentPipelineDraftCompiler{
			newClient: dialContentPipelineDraftCompilerClient,
		},
		fallback: fallback,
	}
}

func contentPipelineDraftsEnabled() bool {
	return envutil.Bool(contentPipelineDraftsEnabledEnv, false)
}

func (c *contentPipelineDraftCompiler) CompileProjectDrafts(ctx context.Context, project *models.Project, publications []models.ProjectPlatformPublication, platforms []string) (map[string][]byte, error) {
	if project == nil {
		return nil, fmt.Errorf("%w: source project is required", errContentPipelineDraftContract)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, contentPipelineDraftTimeout)
	defer cancel()

	client, closer, err := c.newClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect content pipeline draft compiler: %w", err)
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}

	response, err := client.CompileDrafts(ctx, &contentpipelinepb.CompileDraftsRequest{
		RequestId: uuid.NewString(),
		Project: &contentpipelinepb.SourceProject{
			Id:            project.ID.String(),
			Title:         strings.TrimSpace(project.Title),
			SourceFormat:  "html",
			SourceContent: project.SourceContent,
		},
		Targets: draftTargetsForPublications(publications, platforms),
	})
	if err != nil {
		return nil, err
	}

	return compiledDraftsByPlatform(response, platforms)
}

func dialContentPipelineDraftCompilerClient(_ context.Context) (contentpipelinepb.PlatformDraftCompilerClient, io.Closer, error) {
	conn, err := contentpipeline.Dial()
	if err != nil {
		return nil, nil, err
	}
	return contentpipelinepb.NewPlatformDraftCompilerClient(conn), conn, nil
}

func draftTargetsForPublications(publications []models.ProjectPlatformPublication, platforms []string) []*contentpipelinepb.DraftTarget {
	publicationsByPlatform := make(map[string]models.ProjectPlatformPublication, len(publications))
	for _, publication := range publications {
		publicationsByPlatform[publication.Platform] = publication
	}

	targets := make([]*contentpipelinepb.DraftTarget, 0, len(platforms))
	for _, platform := range platforms {
		configJSON := "{}"
		if publication, ok := publicationsByPlatform[platform]; ok && len(publication.Config) > 0 {
			configJSON = string(publication.Config)
		}
		targets = append(targets, &contentpipelinepb.DraftTarget{
			Platform:   platform,
			Profile:    platform + "@v1",
			ConfigJson: configJSON,
		})
	}
	return targets
}

func compiledDraftsByPlatform(response *contentpipelinepb.CompileDraftsResponse, platforms []string) (map[string][]byte, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: missing compile response", errContentPipelineDraftContract)
	}

	requested := make(map[string]struct{}, len(platforms))
	for _, platform := range platforms {
		requested[platform] = struct{}{}
	}

	drafts := make(map[string][]byte, len(platforms))
	for _, draft := range response.GetDrafts() {
		platform := strings.TrimSpace(draft.GetPlatform())
		if platform == "" {
			return nil, fmt.Errorf("%w: compiled draft is missing platform", errContentPipelineDraftContract)
		}
		if _, ok := requested[platform]; !ok {
			return nil, fmt.Errorf("%w: unexpected compiled draft platform %q", errContentPipelineDraftContract, platform)
		}
		if _, exists := drafts[platform]; exists {
			return nil, fmt.Errorf("%w: duplicate compiled draft platform %q", errContentPipelineDraftContract, platform)
		}

		adaptedContent := []byte(strings.TrimSpace(draft.GetAdaptedContentJson()))
		if err := validateCompiledAdaptedContent(platform, adaptedContent); err != nil {
			return nil, err
		}
		drafts[platform] = adaptedContent
	}

	for _, platform := range platforms {
		if _, ok := drafts[platform]; !ok {
			return nil, fmt.Errorf("%w: missing compiled draft for %q", errContentPipelineDraftContract, platform)
		}
	}

	return drafts, nil
}

func validateCompiledAdaptedContent(platform string, adaptedContent []byte) error {
	if len(adaptedContent) == 0 {
		return fmt.Errorf("%w: empty adapted content for %q", errContentPipelineDraftContract, platform)
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(adaptedContent, &payload); err != nil {
		return fmt.Errorf("%w: invalid adapted content JSON for %q: %w", errContentPipelineDraftContract, platform, err)
	}

	expectedFormat := expectedDraftFormat(platform)
	if expectedFormat == "" {
		return nil
	}

	var format string
	if err := json.Unmarshal(payload["format"], &format); err != nil || strings.TrimSpace(format) != expectedFormat {
		return fmt.Errorf("%w: expected %q draft format for %q", errContentPipelineDraftContract, expectedFormat, platform)
	}
	if !hasNonEmptyStringField(payload, expectedFormat) {
		return fmt.Errorf("%w: missing %q field for %q", errContentPipelineDraftContract, expectedFormat, platform)
	}

	return nil
}

func expectedDraftFormat(platform string) string {
	switch platform {
	case "wechat":
		return "html"
	case "zhihu":
		return "markdown"
	case "x", "douyin":
		return "text"
	default:
		return ""
	}
}

func hasNonEmptyStringField(payload map[string]json.RawMessage, field string) bool {
	raw, ok := payload[field]
	if !ok {
		return false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	return strings.TrimSpace(value) != ""
}

type fallbackingDraftCompiler struct {
	primary  ProjectDraftCompiler
	fallback ProjectDraftCompiler
}

func (c *fallbackingDraftCompiler) CompileProjectDrafts(ctx context.Context, project *models.Project, publications []models.ProjectPlatformPublication, platforms []string) (map[string][]byte, error) {
	drafts, err := c.primary.CompileProjectDrafts(ctx, project, publications, platforms)
	if err == nil {
		return drafts, nil
	}
	if !shouldFallbackContentPipelineDraftError(err) {
		return nil, err
	}
	return c.fallback.CompileProjectDrafts(ctx, project, publications, platforms)
}

func shouldFallbackContentPipelineDraftError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errContentPipelineDraftContract) {
		return false
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Unimplemented, codes.Unknown:
		return true
	default:
		return false
	}
}

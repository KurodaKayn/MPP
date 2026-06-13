package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/contracts"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

type AIContextAssembler struct {
	db *gorm.DB
}

func NewAIContextAssembler(db *gorm.DB) *AIContextAssembler {
	return &AIContextAssembler{db: db}
}

type AssembleOptions struct {
	SelectedRange   string
	CompactionLevel string // "none", "partial", "session_summary", "memory_summary"
	ContextBudget   int    // default context budget
}

func (a *AIContextAssembler) Assemble(ctx context.Context, projectID uuid.UUID, createdByID uuid.UUID, kind string, options AssembleOptions) (*models.AIContextSnapshot, error) {
	var project models.Project
	err := a.db.WithContext(ctx).
		Preload("Template").
		Preload("BrandProfile").
		Preload("Publications").
		First(&project, "id = ?", projectID).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch project: %w", err)
	}

	var workspaceID uuid.UUID
	if project.WorkspaceID != nil {
		workspaceID = *project.WorkspaceID
	}

	// Fetch platform accounts
	var accounts []models.PlatformAccount
	if project.WorkspaceID != nil {
		err = a.db.WithContext(ctx).
			Where("workspace_id = ?", *project.WorkspaceID).
			Find(&accounts).Error
		if err != nil {
			return nil, fmt.Errorf("failed to fetch platform accounts: %w", err)
		}
	}

	// Fetch comments
	var comments []models.ProjectComment
	err = a.db.WithContext(ctx).
		Preload("Author").
		Where("project_id = ?", projectID).
		Order("created_at asc").
		Find(&comments).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch comments: %w", err)
	}

	// Fetch versions
	var versions []models.ProjectVersion
	err = a.db.WithContext(ctx).
		Preload("Creator").
		Where("project_id = ?", projectID).
		Order("version_number asc").
		Find(&versions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch project versions: %w", err)
	}

	// Fetch media assets
	var mediaAssets []models.MediaAsset
	err = a.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("created_at asc").
		Find(&mediaAssets).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch media assets: %w", err)
	}

	// Build summaries
	projectSummary := fmt.Sprintf("Project Title: %s\nStatus: %s\nCreated At: %s", project.Title, project.Status, project.CreatedAt.Format(time.RFC3339))

	var commentsList []string
	for _, c := range comments {
		authorEmail := "unknown"
		if c.Author.Email != "" {
			authorEmail = c.Author.Email
		} else if c.Author.Username != "" {
			authorEmail = c.Author.Username
		}
		commentsList = append(commentsList, fmt.Sprintf("Author: %s | Body: %s | Status: %s | CreatedAt: %s", authorEmail, c.Body, c.Status, c.CreatedAt.Format(time.RFC3339)))
	}
	commentsSummary := strings.Join(commentsList, "\n")

	var versionsList []string
	for _, v := range versions {
		creatorEmail := "unknown"
		if v.Creator.Email != "" {
			creatorEmail = v.Creator.Email
		} else if v.Creator.Username != "" {
			creatorEmail = v.Creator.Username
		}
		versionsList = append(versionsList, fmt.Sprintf("Version %d: %s (Created by %s at %s)", v.VersionNumber, v.Title, creatorEmail, v.CreatedAt.Format(time.RFC3339)))
	}
	versionsSummary := strings.Join(versionsList, "\n")

	var mediaList []string
	for _, m := range mediaAssets {
		mediaList = append(mediaList, fmt.Sprintf("Filename: %s | Mime: %s | Size: %d bytes | Alt: %s | Status: %s", m.OriginalFilename, m.MimeType, m.SizeBytes, m.AltText, m.Status))
	}
	mediaSummary := strings.Join(mediaList, "\n")

	var perfList []string
	for _, pub := range project.Publications {
		perfList = append(perfList, fmt.Sprintf("Platform: %s | Status: %s | DraftStatus: %s | PublishURL: %s | Error: %s", pub.Platform, pub.Status, pub.DraftStatus, pub.PublishURL, pub.ErrorMessage))
	}
	performanceSummary := strings.Join(perfList, "\n")

	// Brand profile serialization
	var bpJSON datatypes.JSON
	if project.BrandProfile != nil {
		scrubbedBP, err := structToScrubbedMap(project.BrandProfile)
		if err != nil {
			return nil, fmt.Errorf("failed to scrub brand profile: %w", err)
		}
		bpBytes, err := json.Marshal(scrubbedBP)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal brand profile: %w", err)
		}
		bpJSON = datatypes.JSON(bpBytes)
	}

	// Content template serialization
	var ctJSON datatypes.JSON
	if project.Template != nil {
		scrubbedCT, err := structToScrubbedMap(project.Template)
		if err != nil {
			return nil, fmt.Errorf("failed to scrub content template: %w", err)
		}
		ctBytes, err := json.Marshal(scrubbedCT)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal content template: %w", err)
		}
		ctJSON = datatypes.JSON(ctBytes)
	}

	// Platform accounts serialization
	platformsMap := make(map[string]any)
	for _, acc := range accounts {
		scrubbedAcc, err := structToScrubbedMap(acc)
		if err != nil {
			return nil, fmt.Errorf("failed to scrub platform account: %w", err)
		}
		platformsMap[acc.ID.String()] = scrubbedAcc
	}
	platformsBytes, err := json.Marshal(platformsMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal platform accounts: %w", err)
	}
	platformsJSON := datatypes.JSON(platformsBytes)

	// Publications serialization
	publicationsMap := make(map[string]any)
	for _, pub := range project.Publications {
		scrubbedPub, err := structToScrubbedMap(pub)
		if err != nil {
			return nil, fmt.Errorf("failed to scrub publication: %w", err)
		}
		publicationsMap[pub.ID.String()] = scrubbedPub
	}
	publicationsBytes, err := json.Marshal(publicationsMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal publications: %w", err)
	}
	publicationsJSON := datatypes.JSON(publicationsBytes)

	snapshot := models.AIContextSnapshot{
		ID:                 uuid.New(),
		WorkspaceID:        workspaceID,
		ProjectID:          projectID,
		CreatedByID:        createdByID,
		ContextKind:        kind,
		ProjectSummary:     projectSummary,
		SourceContent:      project.SourceContent,
		SelectedRange:      options.SelectedRange,
		Platforms:          platformsJSON,
		Publications:       publicationsJSON,
		BrandProfile:       bpJSON,
		ContentTemplate:    ctJSON,
		CommentsSummary:    commentsSummary,
		VersionsSummary:    versionsSummary,
		MediaSummary:       mediaSummary,
		PerformanceSummary: performanceSummary,
		CompactionLevel:    options.CompactionLevel,
		CreatedAt:          time.Now(),
	}
	if snapshot.CompactionLevel == "" {
		snapshot.CompactionLevel = "none"
	}

	// Budgeting
	budgeter := NewAIContextBudgeter(options.ContextBudget)
	budgeter.Budget(&snapshot)

	return &snapshot, nil
}

// CreateSnapshot builds and persists a context snapshot.
// NOTE: Assembly reads and persistence are not transactional — the snapshot
// represents a best-effort point-in-time view suitable for AI context.
func (a *AIContextAssembler) CreateSnapshot(ctx context.Context, projectID uuid.UUID, createdByID uuid.UUID, kind string, options AssembleOptions) (*models.AIContextSnapshot, error) {
	snapshot, err := a.Assemble(ctx, projectID, createdByID, kind, options)
	if err != nil {
		return nil, err
	}

	if err := a.db.WithContext(ctx).Create(snapshot).Error; err != nil {
		return nil, fmt.Errorf("failed to save context snapshot: %w", err)
	}

	return snapshot, nil
}

// MapModelToContract maps GORM AIContextSnapshot model to OpenAPI generated structure
func MapModelToContract(m *models.AIContextSnapshot) *contracts.AIContextSnapshot {
	if m == nil {
		return nil
	}

	var brandProfile *map[string]any
	if len(m.BrandProfile) > 0 {
		var bp map[string]any
		if err := json.Unmarshal(m.BrandProfile, &bp); err == nil {
			brandProfile = &bp
		}
	}

	var contentTemplate *map[string]any
	if len(m.ContentTemplate) > 0 {
		var ct map[string]any
		if err := json.Unmarshal(m.ContentTemplate, &ct); err == nil {
			contentTemplate = &ct
		}
	}

	var platforms *map[string]any
	if len(m.Platforms) > 0 {
		var plat map[string]any
		if err := json.Unmarshal(m.Platforms, &plat); err == nil {
			platforms = &plat
		}
	}

	var publications *map[string]any
	if len(m.Publications) > 0 {
		var pub map[string]any
		if err := json.Unmarshal(m.Publications, &pub); err == nil {
			publications = &pub
		}
	}

	var rawContextRefs *map[string]any
	if len(m.RawContextRefs) > 0 {
		var rc map[string]any
		if err := json.Unmarshal(m.RawContextRefs, &rc); err == nil {
			rawContextRefs = &rc
		}
	}

	var sourceVersionId *openapi_types.UUID
	if m.SourceVersionID != nil {
		val := openapi_types.UUID(*m.SourceVersionID)
		sourceVersionId = &val
	}

	compactionLevel := m.CompactionLevel
	if compactionLevel == "" {
		compactionLevel = "none"
	}

	return &contracts.AIContextSnapshot{
		Id:                 openapi_types.UUID(m.ID),
		WorkspaceId:        openapi_types.UUID(m.WorkspaceID),
		ProjectId:          openapi_types.UUID(m.ProjectID),
		CreatedBy:          openapi_types.UUID(m.CreatedByID),
		ContextKind:        contracts.AIContextSnapshotContextKind(m.ContextKind),
		SourceVersionId:    sourceVersionId,
		ProjectSummary:     m.ProjectSummary,
		SourceContent:      m.SourceContent,
		SelectedRange:      m.SelectedRange,
		BrandProfile:       brandProfile,
		ContentTemplate:    contentTemplate,
		Platforms:          platforms,
		Publications:       publications,
		CommentsSummary:    m.CommentsSummary,
		VersionsSummary:    m.VersionsSummary,
		MediaSummary:       m.MediaSummary,
		PerformanceSummary: m.PerformanceSummary,
		TokenEstimate:      m.TokenEstimate,
		ContextBudget:      m.ContextBudget,
		CompactionLevel:    contracts.AIContextSnapshotCompactionLevel(compactionLevel),
		RawContextRefs:     rawContextRefs,
		CreatedAt:          m.CreatedAt,
	}
}

// AIContextBudgeter estimates and compacts snapshot context
type AIContextBudgeter struct {
	MaxBudget int
}

func NewAIContextBudgeter(maxBudget int) *AIContextBudgeter {
	if maxBudget <= 0 {
		maxBudget = 100000 // default budget
	}
	return &AIContextBudgeter{MaxBudget: maxBudget}
}

func EstimateTokens(text string) int {
	asciiCount := 0
	nonAsciiCount := 0
	for _, r := range text {
		if r <= 127 {
			asciiCount++
		} else {
			nonAsciiCount++
		}
	}
	// ASCII: ~4 chars per token, Non-ASCII: ~2 tokens per character (round up ASCII to be conservative)
	return ((asciiCount + 3) / 4) + (nonAsciiCount * 2)
}

func (b *AIContextBudgeter) sumTokens(snapshot *models.AIContextSnapshot) int {
	totalTokens := 0
	totalTokens += EstimateTokens(snapshot.SourceContent)
	totalTokens += EstimateTokens(snapshot.ProjectSummary)
	totalTokens += EstimateTokens(snapshot.CommentsSummary)
	totalTokens += EstimateTokens(snapshot.VersionsSummary)
	totalTokens += EstimateTokens(snapshot.MediaSummary)
	totalTokens += EstimateTokens(snapshot.PerformanceSummary)
	totalTokens += EstimateTokens(snapshot.SelectedRange)

	totalTokens += estimateJSONTokens(snapshot.Platforms)
	totalTokens += estimateJSONTokens(snapshot.Publications)
	totalTokens += estimateJSONTokens(snapshot.BrandProfile)
	totalTokens += estimateJSONTokens(snapshot.ContentTemplate)
	totalTokens += estimateJSONTokens(snapshot.RawContextRefs)
	return totalTokens
}

func (b *AIContextBudgeter) Budget(snapshot *models.AIContextSnapshot) int {
	totalTokens := b.sumTokens(snapshot)
	snapshot.TokenEstimate = totalTokens
	snapshot.ContextBudget = b.MaxBudget

	if totalTokens > b.MaxBudget {
		b.TruncateIfNeeded(snapshot)
	}

	return snapshot.TokenEstimate
}

func (b *AIContextBudgeter) TruncateIfNeeded(snapshot *models.AIContextSnapshot) {
	if EstimateTokens(snapshot.CommentsSummary) > 1250 {
		snapshot.CommentsSummary = truncateString(snapshot.CommentsSummary, 5000)
		snapshot.CompactionLevel = "partial"
	}
	if EstimateTokens(snapshot.VersionsSummary) > 1250 {
		snapshot.VersionsSummary = truncateString(snapshot.VersionsSummary, 5000)
		snapshot.CompactionLevel = "partial"
	}
	if EstimateTokens(snapshot.MediaSummary) > 1250 {
		snapshot.MediaSummary = truncateString(snapshot.MediaSummary, 5000)
		snapshot.CompactionLevel = "partial"
	}
	if EstimateTokens(snapshot.PerformanceSummary) > 1250 {
		snapshot.PerformanceSummary = truncateString(snapshot.PerformanceSummary, 5000)
		snapshot.CompactionLevel = "partial"
	}

	// Recalculate
	totalTokens := b.sumTokens(snapshot)
	snapshot.TokenEstimate = totalTokens

	if totalTokens > b.MaxBudget {
		// Truncate source content if still over budget
		runes := []rune(snapshot.SourceContent)
		if len(runes) > 20000 {
			snapshot.SourceContent = truncateString(snapshot.SourceContent, 20000)
			snapshot.CompactionLevel = "partial"
		}

		// Recalculate once more
		snapshot.TokenEstimate = b.sumTokens(snapshot)
	}
}

func truncateString(s string, maxChars int) string {
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	half := maxChars / 2
	return string(runes[:half]) + "\n... [TRUNCATED] ...\n" + string(runes[len(runes)-half:])
}

func estimateJSONTokens(j datatypes.JSON) int {
	if len(j) == 0 {
		return 0
	}
	return EstimateTokens(string(j))
}

// Security scrubbing layer helpers
func ScrubMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		kLower := strings.ToLower(k)
		if isSensitiveKey(kLower) {
			result[k] = "[REDACTED]"
			continue
		}
		result[k] = scrubValue(v)
	}
	return result
}

func scrubValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return ScrubMap(val)
	case []any:
		result := make([]any, len(val))
		for i, elem := range val {
			result[i] = scrubValue(elem)
		}
		return result
	default:
		return v
	}
}

func isSensitiveKey(k string) bool {
	sensitiveFields := map[string]bool{
		"password":              true,
		"pass":                  true,
		"pwd":                   true,
		"secret":                true,
		"credential":            true,
		"credentials":           true,
		"access_token":          true,
		"refresh_token":         true,
		"jwt":                   true,
		"session":               true,
		"cookie":                true,
		"cookies":               true,
		"credential_secret_ref": true,
		"app_secret":            true,
		"api_secret":            true,
		"token":                 true,
	}
	return sensitiveFields[k]
}

func structToScrubbedMap(s any) (map[string]any, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return ScrubMap(raw), nil
}

package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/compiler"
)

const (
	defaultGrowthContextBudget = 50000
	defaultGrowthModel         = "growth-optimizer"
	defaultGrowthPromptVersion = "growth-v1"
)

var ErrInvalidGrowthOptimizationRequest = errors.New("invalid growth optimization request")

type GrowthOptimizationService struct {
	db            *gorm.DB
	assembler     *AIContextAssembler
	optimizer     GrowthOptimizer
	draftCompiler compiler.ProjectDraftCompiler
}

func NewGrowthOptimizationService(db *gorm.DB, optimizer GrowthOptimizer) *GrowthOptimizationService {
	return &GrowthOptimizationService{
		db:            db,
		assembler:     NewAIContextAssembler(db),
		optimizer:     optimizer,
		draftCompiler: compiler.NewContentPipelineDraftCompiler(),
	}
}

func (s *GrowthOptimizationService) SetDraftCompiler(draftCompiler compiler.ProjectDraftCompiler) {
	if s == nil {
		return
	}
	s.draftCompiler = draftCompiler
}

type GrowthOptimizationProposalEvent struct {
	ProposalType   string         `json:"proposal_type"`
	TargetPlatform string         `json:"target_platform"`
	Summary        string         `json:"summary"`
	Patch          string         `json:"patch"`
	FullContent    string         `json:"full_content"`
	QualityChecks  map[string]any `json:"quality_checks"`
}

type GrowthOptimizationStatusEvent struct {
	Status         string         `json:"status"`
	Model          string         `json:"model"`
	PromptVersion  string         `json:"prompt_version"`
	QualitySummary string         `json:"quality_summary"`
	Usage          map[string]any `json:"usage"`
}

func (s *GrowthOptimizationService) CreateRun(ctx context.Context, projectID, userID uuid.UUID, req dto.CreateAIGrowthOptimizationRunRequest) (*dto.AIGrowthOptimizationRunResponse, error) {
	if s == nil || s.optimizer == nil {
		return nil, ErrAIServiceUnavailable
	}
	req.Goal = strings.TrimSpace(req.Goal)
	req.Intensity = normalizeGrowthIntensity(req.Intensity)
	req.TargetPlatforms = normalizeGrowthPlatforms(req.TargetPlatforms)
	if projectID == uuid.Nil || userID == uuid.Nil || req.Goal == "" || len(req.TargetPlatforms) == 0 {
		return nil, ErrInvalidGrowthOptimizationRequest
	}

	snapshot, err := s.assembler.CreateSnapshot(ctx, projectID, userID, "growth_optimization", AssembleOptions{
		ContextBudget: defaultGrowthContextBudget,
	})
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.Title) == "" {
		req.Title = titleFromProjectSummary(snapshot.ProjectSummary)
	}
	if strings.TrimSpace(req.SourceContent) == "" {
		req.SourceContent = snapshot.SourceContent
	}
	if strings.TrimSpace(req.SourceContent) == "" {
		return nil, ErrInvalidGrowthOptimizationRequest
	}

	targetPlatformsJSON, err := json.Marshal(req.TargetPlatforms)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	run := models.AIGrowthOptimizationRun{
		ID:                uuid.New(),
		WorkspaceID:       snapshot.WorkspaceID,
		ProjectID:         projectID,
		ContextSnapshotID: snapshot.ID,
		Goal:              req.Goal,
		Intensity:         req.Intensity,
		TargetPlatforms:   datatypes.JSON(targetPlatformsJSON),
		Status:            "running",
		Model:             defaultGrowthModel,
		PromptVersion:     defaultGrowthPromptVersion,
		CreatedByID:       userID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.db.WithContext(ctx).Create(&run).Error; err != nil {
		return nil, err
	}

	stream, err := s.optimizer.StreamGrowthOptimization(ctx, req)
	if err != nil {
		s.markRunTerminal(context.WithoutCancel(ctx), &run, terminalGrowthRunStatus(err), err.Error())
		return nil, err
	}
	defer func() { _ = stream.Body.Close() }()

	proposalEvents, status, err := readGrowthOptimizationEvents(stream.Body)
	if err != nil {
		s.markRunTerminal(context.WithoutCancel(ctx), &run, terminalGrowthRunStatus(err), err.Error())
		return nil, err
	}
	if status.Model != "" {
		run.Model = status.Model
	}
	if status.PromptVersion != "" {
		run.PromptVersion = status.PromptVersion
	}
	run.QualitySummary = status.QualitySummary
	run.Usage = jsonMap(status.Usage)
	run.Status = "ready"
	run.UpdatedAt = time.Now()

	publications, err := s.growthCandidatePublications(ctx, projectID, req.TargetPlatforms)
	if err != nil {
		s.markRunTerminal(context.WithoutCancel(ctx), &run, "failed", err.Error())
		return nil, err
	}

	requestedPlatforms := make(map[string]struct{}, len(req.TargetPlatforms))
	for _, platform := range req.TargetPlatforms {
		requestedPlatforms[platform] = struct{}{}
	}

	var proposals []models.AIProposal
	for _, event := range proposalEvents {
		event.TargetPlatform = strings.TrimSpace(event.TargetPlatform)
		event.ProposalType = strings.TrimSpace(event.ProposalType)
		if event.ProposalType == "" || event.TargetPlatform == "" {
			continue
		}
		if _, ok := requestedPlatforms[event.TargetPlatform]; !ok {
			continue
		}
		if event.ProposalType == "prepublish_patch" {
			adaptedContent, err := s.compileGrowthCandidate(ctx, projectID, req.Title, event, publications)
			if err != nil {
				continue
			}
			event.QualityChecks = withContentPipelineQualityChecks(event.QualityChecks, adaptedContent)
		}

		proposal := models.AIProposal{
			ID:                uuid.New(),
			WorkspaceID:       snapshot.WorkspaceID,
			ProjectID:         projectID,
			RunID:             &run.ID,
			ContextSnapshotID: snapshot.ID,
			ProposalType:      event.ProposalType,
			TargetPlatform:    event.TargetPlatform,
			Status:            "proposed",
			Summary:           strings.TrimSpace(event.Summary),
			Patch:             event.Patch,
			FullContent:       event.FullContent,
			QualityChecks:     jsonMap(event.QualityChecks),
			CreatedAt:         time.Now(),
		}
		proposals = append(proposals, proposal)
	}
	if len(proposals) == 0 {
		s.markRunTerminal(context.WithoutCancel(ctx), &run, "failed", "growth optimizer returned no compilable proposals")
		return nil, fmt.Errorf("%w: no compilable proposals returned", ErrAIServiceUnavailable)
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&run).Error; err != nil {
			return err
		}
		return tx.Create(&proposals).Error
	})
	if err != nil {
		return nil, err
	}

	return mapGrowthRunResponse(run, proposals)
}

func (s *GrowthOptimizationService) growthCandidatePublications(ctx context.Context, projectID uuid.UUID, platforms []string) ([]models.ProjectPlatformPublication, error) {
	var publications []models.ProjectPlatformPublication
	if err := s.db.WithContext(ctx).
		Where("project_id = ? AND platform IN ?", projectID, platforms).
		Find(&publications).Error; err != nil {
		return nil, err
	}
	return publications, nil
}

func (s *GrowthOptimizationService) compileGrowthCandidate(ctx context.Context, projectID uuid.UUID, title string, event GrowthOptimizationProposalEvent, publications []models.ProjectPlatformPublication) ([]byte, error) {
	draftCompiler := s.draftCompiler
	if draftCompiler == nil {
		draftCompiler = compiler.NewContentPipelineDraftCompiler()
	}

	project := &models.Project{
		ID:            projectID,
		Title:         strings.TrimSpace(title),
		SourceContent: strings.TrimSpace(event.FullContent),
	}
	compiled, err := draftCompiler.CompileProjectDrafts(ctx, project, publications, []string{event.TargetPlatform})
	if err != nil {
		return nil, err
	}
	adaptedContent := compiled[event.TargetPlatform]
	if len(adaptedContent) == 0 {
		return nil, fmt.Errorf("content pipeline did not compile %q growth candidate", event.TargetPlatform)
	}
	return adaptedContent, nil
}

func withContentPipelineQualityChecks(qualityChecks map[string]any, adaptedContent []byte) map[string]any {
	if qualityChecks == nil {
		qualityChecks = make(map[string]any, 2)
	}
	qualityChecks["content_pipeline_status"] = "compiled"
	qualityChecks["content_pipeline_adapted_content"] = json.RawMessage(adaptedContent)
	return qualityChecks
}

func (s *GrowthOptimizationService) markRunTerminal(ctx context.Context, run *models.AIGrowthOptimizationRun, status string, message string) {
	if run == nil || run.ID == uuid.Nil {
		return
	}
	if status == "" {
		status = "failed"
	}
	if err := s.db.WithContext(ctx).Model(run).Updates(map[string]any{
		"status":          status,
		"quality_summary": strings.TrimSpace(message),
		"updated_at":      time.Now(),
	}).Error; err != nil {
		log.Printf("[ai] failed to mark growth run terminal run=%s status=%s: %v", run.ID, status, err)
	}
}

func terminalGrowthRunStatus(err error) string {
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	return "failed"
}

func readGrowthOptimizationEvents(body io.Reader) ([]GrowthOptimizationProposalEvent, GrowthOptimizationStatusEvent, error) {
	var proposals []GrowthOptimizationProposalEvent
	var status GrowthOptimizationStatusEvent
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	eventType := ""
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			switch eventType {
			case "proposal":
				var event GrowthOptimizationProposalEvent
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					return nil, status, err
				}
				proposals = append(proposals, event)
			case "status":
				var event GrowthOptimizationStatusEvent
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					return nil, status, err
				}
				if event.Status == "ready" {
					status = event
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, status, err
	}
	return proposals, status, nil
}

func normalizeGrowthIntensity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "conservative", "aggressive":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "balanced"
	}
}

func normalizeGrowthPlatforms(values []string) []string {
	seen := make(map[string]bool, len(values))
	var out []string
	for _, value := range values {
		platform := strings.ToLower(strings.TrimSpace(value))
		if platform == "" || seen[platform] {
			continue
		}
		seen[platform] = true
		out = append(out, platform)
	}
	return out
}

func titleFromProjectSummary(summary string) string {
	for line := range strings.SplitSeq(summary, "\n") {
		if title, ok := strings.CutPrefix(line, "Project Title: "); ok {
			return strings.TrimSpace(title)
		}
	}
	return ""
}

func jsonMap(value map[string]any) datatypes.JSON {
	if len(value) == 0 {
		return datatypes.JSON([]byte(`{}`))
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return datatypes.JSON([]byte(`{}`))
	}
	return datatypes.JSON(raw)
}

func mapGrowthRunResponse(run models.AIGrowthOptimizationRun, proposals []models.AIProposal) (*dto.AIGrowthOptimizationRunResponse, error) {
	var targetPlatforms []string
	if err := unmarshalJSONField(run.TargetPlatforms, &targetPlatforms); err != nil {
		return nil, fmt.Errorf("decode growth run target platforms: %w", err)
	}
	var usage map[string]any
	if err := unmarshalJSONField(run.Usage, &usage); err != nil {
		return nil, fmt.Errorf("decode growth run usage: %w", err)
	}
	out := &dto.AIGrowthOptimizationRunResponse{
		Run: dto.AIGrowthOptimizationRun{
			ID:                run.ID,
			WorkspaceID:       run.WorkspaceID,
			ProjectID:         run.ProjectID,
			ContextSnapshotID: run.ContextSnapshotID,
			Goal:              run.Goal,
			Intensity:         run.Intensity,
			TargetPlatforms:   targetPlatforms,
			Status:            run.Status,
			Model:             run.Model,
			PromptVersion:     run.PromptVersion,
			Usage:             usage,
			QualitySummary:    run.QualitySummary,
			CreatedBy:         run.CreatedByID,
			CreatedAt:         run.CreatedAt,
			UpdatedAt:         run.UpdatedAt,
		},
		Proposals: make([]dto.AIProposal, 0, len(proposals)),
	}
	for _, proposal := range proposals {
		var qualityChecks map[string]any
		if err := unmarshalJSONField(proposal.QualityChecks, &qualityChecks); err != nil {
			return nil, fmt.Errorf("decode proposal quality checks: %w", err)
		}
		out.Proposals = append(out.Proposals, dto.AIProposal{
			ID:                proposal.ID,
			WorkspaceID:       proposal.WorkspaceID,
			ProjectID:         proposal.ProjectID,
			RunID:             proposal.RunID,
			ContextSnapshotID: proposal.ContextSnapshotID,
			ProposalType:      proposal.ProposalType,
			TargetPlatform:    proposal.TargetPlatform,
			Status:            proposal.Status,
			Summary:           proposal.Summary,
			Patch:             proposal.Patch,
			FullContent:       proposal.FullContent,
			QualityChecks:     qualityChecks,
			CreatedAt:         proposal.CreatedAt,
		})
	}
	return out, nil
}

func unmarshalJSONField(raw datatypes.JSON, out any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

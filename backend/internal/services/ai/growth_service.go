package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

const (
	defaultGrowthContextBudget = 50000
	defaultGrowthModel         = "growth-optimizer"
	defaultGrowthPromptVersion = "growth-v1"
)

var ErrInvalidGrowthOptimizationRequest = errors.New("invalid growth optimization request")

type GrowthOptimizationService struct {
	db        *gorm.DB
	assembler *AIContextAssembler
	optimizer GrowthOptimizer
}

func NewGrowthOptimizationService(db *gorm.DB, optimizer GrowthOptimizer) *GrowthOptimizationService {
	return &GrowthOptimizationService{
		db:        db,
		assembler: NewAIContextAssembler(db),
		optimizer: optimizer,
	}
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
		s.markRunFailed(context.WithoutCancel(ctx), &run, err.Error())
		return nil, err
	}
	defer func() { _ = stream.Body.Close() }()

	proposalEvents, status, err := readGrowthOptimizationEvents(stream.Body)
	if err != nil {
		s.markRunFailed(context.WithoutCancel(ctx), &run, err.Error())
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

	var proposals []models.AIProposal
	for _, event := range proposalEvents {
		proposal := models.AIProposal{
			ID:                uuid.New(),
			WorkspaceID:       snapshot.WorkspaceID,
			ProjectID:         projectID,
			RunID:             &run.ID,
			ContextSnapshotID: snapshot.ID,
			ProposalType:      strings.TrimSpace(event.ProposalType),
			TargetPlatform:    strings.TrimSpace(event.TargetPlatform),
			Status:            "proposed",
			Summary:           strings.TrimSpace(event.Summary),
			Patch:             event.Patch,
			FullContent:       event.FullContent,
			QualityChecks:     jsonMap(event.QualityChecks),
			CreatedAt:         time.Now(),
		}
		if proposal.ProposalType == "" || proposal.TargetPlatform == "" {
			continue
		}
		proposals = append(proposals, proposal)
	}
	if len(proposals) == 0 {
		s.markRunFailed(context.WithoutCancel(ctx), &run, "growth optimizer returned no proposals")
		return nil, fmt.Errorf("%w: no proposals returned", ErrAIServiceUnavailable)
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

	return mapGrowthRunResponse(run, proposals), nil
}

func (s *GrowthOptimizationService) markRunFailed(ctx context.Context, run *models.AIGrowthOptimizationRun, message string) {
	if run == nil || run.ID == uuid.Nil {
		return
	}
	_ = s.db.WithContext(ctx).Model(run).Updates(map[string]any{
		"status":          "failed",
		"quality_summary": strings.TrimSpace(message),
		"updated_at":      time.Now(),
	}).Error
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

func mapGrowthRunResponse(run models.AIGrowthOptimizationRun, proposals []models.AIProposal) *dto.AIGrowthOptimizationRunResponse {
	var targetPlatforms []string
	_ = json.Unmarshal(run.TargetPlatforms, &targetPlatforms)
	var usage map[string]any
	_ = json.Unmarshal(run.Usage, &usage)
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
		_ = json.Unmarshal(proposal.QualityChecks, &qualityChecks)
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
	return out
}

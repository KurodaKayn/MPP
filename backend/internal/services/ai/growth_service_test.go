package ai

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

type fakeGrowthOptimizer struct {
	lastRequest dto.CreateAIGrowthOptimizationRunRequest
	body        string
	err         error
}

func (f *fakeGrowthOptimizer) StreamGrowthOptimization(_ context.Context, req dto.CreateAIGrowthOptimizationRunRequest) (*AIServiceStream, error) {
	f.lastRequest = req
	if f.err != nil {
		return nil, f.err
	}
	return &AIServiceStream{
		Body:        io.NopCloser(strings.NewReader(f.body)),
		ContentType: "text/event-stream; charset=utf-8",
	}, nil
}

func TestGrowthOptimizationServiceCreatesReadyRunAndProposal(t *testing.T) {
	db := testsupport.SetupTestDB()
	require.NoError(t, db.AutoMigrate(
		&models.AIContextSnapshot{},
		&models.AIGrowthOptimizationRun{},
		&models.AIProposal{},
		&models.ProjectComment{},
		&models.MediaAsset{},
	))

	userID := uuid.New()
	workspaceID := uuid.New()
	projectID := uuid.New()
	require.NoError(t, db.Create(&models.User{
		ID:           userID,
		Username:     "growth-user",
		Email:        "growth@example.com",
		PasswordHash: "hash",
	}).Error)
	require.NoError(t, db.Create(&models.Workspace{
		ID:          workspaceID,
		OwnerUserID: userID,
		Name:        "Growth Workspace",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}).Error)
	require.NoError(t, db.Create(&models.Project{
		ID:            projectID,
		UserID:        userID,
		WorkspaceID:   &workspaceID,
		Title:         "Launch note",
		SourceContent: "Original article",
		Status:        models.ProjectStatusDraft,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ID:        uuid.New(),
		ProjectID: projectID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusDraft,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}).Error)

	optimizer := &fakeGrowthOptimizer{body: strings.Join([]string{
		`event: status`,
		`data: {"status":"running","prompt_version":"growth-v1"}`,
		``,
		`event: proposal`,
		`data: {"proposal_type":"prepublish_patch","target_platform":"wechat","summary":"Wechat proposal","patch":"","full_content":"Optimized draft","quality_checks":{"audience_profile":"wechat@growth-v1"}}`,
		``,
		`event: status`,
		`data: {"status":"ready","model":"test-model","prompt_version":"growth-v1","quality_summary":"Review before applying","usage":{"total_tokens":12}}`,
		``,
	}, "\n")}

	service := NewGrowthOptimizationService(db, optimizer)
	resp, err := service.CreateRun(t.Context(), projectID, userID, dto.CreateAIGrowthOptimizationRunRequest{
		Goal:            "improve platform fit",
		TargetPlatforms: []string{"wechat"},
	})

	require.NoError(t, err)
	require.Equal(t, "ready", resp.Run.Status)
	require.Equal(t, "test-model", resp.Run.Model)
	require.Equal(t, []string{"wechat"}, resp.Run.TargetPlatforms)
	require.Len(t, resp.Proposals, 1)
	require.Equal(t, "prepublish_patch", resp.Proposals[0].ProposalType)
	require.Equal(t, "wechat@growth-v1", resp.Proposals[0].QualityChecks["audience_profile"])
	require.Equal(t, "Launch note", optimizer.lastRequest.Title)
	require.Equal(t, "Original article", optimizer.lastRequest.SourceContent)

	var persistedRun models.AIGrowthOptimizationRun
	require.NoError(t, db.First(&persistedRun, "id = ?", resp.Run.ID).Error)
	require.Equal(t, "ready", persistedRun.Status)

	var persistedProposals []models.AIProposal
	require.NoError(t, db.Where("run_id = ?", resp.Run.ID).Find(&persistedProposals).Error)
	require.Len(t, persistedProposals, 1)
}

func TestGrowthOptimizationServiceMarksCancelledRun(t *testing.T) {
	db := testsupport.SetupTestDB()
	require.NoError(t, db.AutoMigrate(
		&models.AIContextSnapshot{},
		&models.AIGrowthOptimizationRun{},
		&models.AIProposal{},
		&models.ProjectComment{},
		&models.MediaAsset{},
	))

	userID := uuid.New()
	workspaceID := uuid.New()
	projectID := uuid.New()
	require.NoError(t, db.Create(&models.User{
		ID:           userID,
		Username:     "cancel-user",
		Email:        "cancel@example.com",
		PasswordHash: "hash",
	}).Error)
	require.NoError(t, db.Create(&models.Workspace{
		ID:          workspaceID,
		OwnerUserID: userID,
		Name:        "Cancel Workspace",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}).Error)
	require.NoError(t, db.Create(&models.Project{
		ID:            projectID,
		UserID:        userID,
		WorkspaceID:   &workspaceID,
		Title:         "Cancelled note",
		SourceContent: "Original article",
		Status:        models.ProjectStatusDraft,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}).Error)

	service := NewGrowthOptimizationService(db, &fakeGrowthOptimizer{err: context.Canceled})
	_, err := service.CreateRun(t.Context(), projectID, userID, dto.CreateAIGrowthOptimizationRunRequest{
		Goal:            "improve platform fit",
		TargetPlatforms: []string{"wechat"},
	})

	require.ErrorIs(t, err, context.Canceled)

	var persistedRun models.AIGrowthOptimizationRun
	require.NoError(t, db.First(&persistedRun, "project_id = ?", projectID).Error)
	require.Equal(t, "cancelled", persistedRun.Status)
}

func TestMapGrowthRunResponseRejectsCorruptJSON(t *testing.T) {
	_, err := mapGrowthRunResponse(models.AIGrowthOptimizationRun{
		ID:              uuid.New(),
		TargetPlatforms: datatypes.JSON(`not-json`),
	}, nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "decode growth run target platforms")
}

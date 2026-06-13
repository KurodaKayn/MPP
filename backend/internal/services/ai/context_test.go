package ai

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestAIContextAssemblerAndBudgeter(t *testing.T) {
	db := testsupport.SetupTestDB()
	err := db.AutoMigrate(
		&models.AIContextSnapshot{},
		&models.AIGrowthOptimizationRun{},
		&models.AIProposal{},
		&models.AIDraftingSession{},
		&models.AIDraftingMessage{},
		&models.AIToolCall{},
		&models.AIDraftingSessionSummary{},
		&models.AISessionEvent{},
		&models.MediaAsset{},
		&models.ProjectComment{},
	)
	require.NoError(t, err)

	// Create test entities
	workspaceID := uuid.New()
	userID := uuid.New()
	projectID := uuid.New()

	user := models.User{
		ID:           userID,
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, db.Create(&user).Error)

	workspace := models.Workspace{
		ID:          workspaceID,
		OwnerUserID: userID,
		Name:        "Test Workspace",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&workspace).Error)

	brandProfile := models.BrandProfile{
		ID:          uuid.New(),
		WorkspaceID: workspaceID,
		CreatedBy:   userID,
		Name:        "Test Brand",
		Voice:       "Professional",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&brandProfile).Error)

	contentTemplate := models.ContentTemplate{
		ID:          uuid.New(),
		WorkspaceID: &workspaceID,
		OwnerUserID: &userID,
		Scope:       "workspace",
		Name:        "Default Temp",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&contentTemplate).Error)

	project := models.Project{
		ID:             projectID,
		UserID:         userID,
		WorkspaceID:    &workspaceID,
		BrandProfileID: &brandProfile.ID,
		TemplateID:     &contentTemplate.ID,
		Title:          "Growth Strategy",
		SourceContent:  "Write an amazing post about web development tools.",
		Status:         "draft",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, db.Create(&project).Error)

	// PlatformAccount with credentials to verify scrubbing
	platformAccount := models.PlatformAccount{
		ID:                  uuid.New(),
		UserID:              userID,
		WorkspaceID:         &workspaceID,
		Platform:            "wechat",
		Username:            "wechat_user",
		CredentialSecretRef: "secret-ref-123",
		Credentials:         datatypes.JSON(`{"app_secret":"my-super-secret-key-123"}`),
		Cookies:             datatypes.JSON(`[{"name":"session_cookie","value":"abc123token"}]`),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}
	require.NoError(t, db.Create(&platformAccount).Error)

	// Publication
	publication := models.ProjectPlatformPublication{
		ID:        uuid.New(),
		ProjectID: projectID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    "draft",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&publication).Error)

	// Comment
	comment := models.ProjectComment{
		ID:        uuid.New(),
		ProjectID: projectID,
		AuthorID:  userID,
		Body:      "Please polish the second paragraph.",
		Status:    "open",
		CreatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&comment).Error)

	// Version
	version := models.ProjectVersion{
		ID:            uuid.New(),
		ProjectID:     projectID,
		CreatedBy:     userID,
		VersionNumber: 1,
		Title:         "Initial Draft",
		SourceContent: "Draft content",
		Source:        "manual",
		CreatedAt:     time.Now(),
	}
	require.NoError(t, db.Create(&version).Error)

	// Media Asset
	mediaAsset := models.MediaAsset{
		ID:               uuid.New(),
		UserID:           userID,
		ProjectID:        &projectID,
		Bucket:           "bucket",
		ObjectKey:        "key/post.png",
		OriginalFilename: "post.png",
		MimeType:         "image/png",
		SizeBytes:        1024,
		LibraryScope:     "project",
		Status:           "ready",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	require.NoError(t, db.Create(&mediaAsset).Error)

	// Assemble Context Snapshot
	assembler := NewAIContextAssembler(db)
	ctx := context.Background()

	t.Run("Scrubbing Verification", func(t *testing.T) {
		snapshot, err := assembler.Assemble(ctx, projectID, userID, "drafting", AssembleOptions{
			ContextBudget: 100000,
		})
		require.NoError(t, err)
		assert.Equal(t, "drafting", snapshot.ContextKind)
		assert.Contains(t, snapshot.ProjectSummary, "Growth Strategy")

		// Verify credentials are scrubbed
		platformsStr := string(snapshot.Platforms)
		assert.Contains(t, platformsStr, "[REDACTED]")
		assert.NotContains(t, platformsStr, "my-super-secret-key-123")
		assert.NotContains(t, platformsStr, "abc123token")

		// Verify map representation conversion
		contract := MapModelToContract(snapshot)
		require.NotNil(t, contract)
		assert.Equal(t, string(snapshot.ID.String()), contract.Id.String())
	})

	t.Run("Context Budgeting and Truncation", func(t *testing.T) {
		// Mock massive comment log to trigger truncation
		largeComments := make([]string, 500)
		for i := range 500 {
			largeComments[i] = "This is comment item to waste budget token limit size. Please review."
		}
		hugeCommentStr := strings.Join(largeComments, "\n")

		snapshot := models.AIContextSnapshot{
			ID:              uuid.New(),
			WorkspaceID:     workspaceID,
			ProjectID:       projectID,
			CreatedByID:     userID,
			ContextKind:     "drafting",
			CommentsSummary: hugeCommentStr,
			SourceContent:   "Small source",
			CompactionLevel: "none",
		}

		budgeter := NewAIContextBudgeter(2000) // Small budget
		tokens := budgeter.Budget(&snapshot)

		assert.Equal(t, "partial", snapshot.CompactionLevel)
		assert.Contains(t, snapshot.CommentsSummary, "[TRUNCATED]")
		assert.Less(t, tokens, 3000)
	})
}

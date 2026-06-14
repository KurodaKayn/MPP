package ai

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestQuotaServiceRecordAndCheck(t *testing.T) {
	db := testsupport.SetupTestDB()
	require.NoError(t, db.AutoMigrate(
		&models.AIUsageRecord{},
		&models.WorkspaceQuotaAggregate{},
	))

	ctx := context.Background()
	workspaceID := uuid.New()
	userID := uuid.New()

	// Seed workspace so FK constraint is satisfied.
	workspace := models.Workspace{
		ID:          workspaceID,
		OwnerUserID: userID,
		Name:        "Test Workspace",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	// User needed for workspace FK.
	user := models.User{
		ID:           userID,
		Username:     "quotauser",
		Email:        "quota@test.com",
		PasswordHash: "hash",
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&workspace).Error)

	svc := NewQuotaService(db)

	t.Run("within quota when no usage recorded", func(t *testing.T) {
		err := svc.CheckQuota(ctx, workspaceID)
		assert.NoError(t, err)
	})

	t.Run("RecordUsage writes record and updates aggregate", func(t *testing.T) {
		usage := &dto.AIUsage{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
			Cost:         0.0003,
			Currency:     "USD",
		}
		err := svc.RecordUsage(ctx, workspaceID, userID, nil, "drafting", usage)
		require.NoError(t, err)

		agg, err := svc.GetAggregate(ctx, workspaceID)
		require.NoError(t, err)
		assert.Equal(t, int64(150), agg.TotalTokens)
		assert.InDelta(t, 0.0003, agg.TotalCost, 1e-9)
	})

	t.Run("aggregate accumulates across multiple calls", func(t *testing.T) {
		usage := &dto.AIUsage{
			InputTokens:  200,
			OutputTokens: 100,
			TotalTokens:  300,
			Cost:         0.0006,
			Currency:     "USD",
		}
		require.NoError(t, svc.RecordUsage(ctx, workspaceID, userID, nil, "drafting", usage))

		agg, err := svc.GetAggregate(ctx, workspaceID)
		require.NoError(t, err)
		// 150 + 300 = 450
		assert.Equal(t, int64(450), agg.TotalTokens)
	})

	t.Run("ErrQuotaExceeded when over limit", func(t *testing.T) {
		// Create a service with a very small limit (1 token).
		tightSvc := &QuotaService{db: db, limitTokens: 1}
		err := tightSvc.CheckQuota(ctx, workspaceID)
		assert.ErrorIs(t, err, ErrQuotaExceeded)
	})

	t.Run("RecordUsage is no-op for nil usage", func(t *testing.T) {
		err := svc.RecordUsage(ctx, workspaceID, userID, nil, "drafting", nil)
		assert.NoError(t, err)
	})
}

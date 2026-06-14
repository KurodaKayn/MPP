package ai

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

const (
	aiQuotaLimitTokensEnv   = "AI_QUOTA_LIMIT_TOKENS" //nolint:gosec // env var name, not a credential
	defaultQuotaLimitTokens = int64(10_000_000)
)

// ErrQuotaExceeded is returned by QuotaService.CheckQuota when the
// workspace has consumed more tokens than the configured limit.
var ErrQuotaExceeded = errors.New("workspace AI token quota exceeded")

// QuotaService manages per-workspace AI usage recording and quota gating.
type QuotaService struct {
	db          *gorm.DB
	limitTokens int64
}

func NewQuotaService(db *gorm.DB) *QuotaService {
	return &QuotaService{
		db:          db,
		limitTokens: quotaLimitFromEnv(),
	}
}

// CheckQuota returns ErrQuotaExceeded if the workspace total token usage
// meets or exceeds the configured limit. Returns nil if within quota or
// if no aggregate row exists yet (first call).
func (q *QuotaService) CheckQuota(ctx context.Context, workspaceID uuid.UUID) error {
	var agg models.WorkspaceQuotaAggregate
	err := q.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		First(&agg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil // no usage yet → within quota
	}
	if err != nil {
		return fmt.Errorf("quota check failed: %w", err)
	}
	if agg.TotalTokens >= q.limitTokens {
		return fmt.Errorf("%w: used=%d limit=%d", ErrQuotaExceeded, agg.TotalTokens, q.limitTokens)
	}
	return nil
}

// RecordUsage persists a single AI call's real provider usage and
// atomically increments the workspace aggregate. It is best-effort:
// failures are logged but do not abort the caller's flow.
func (q *QuotaService) RecordUsage(ctx context.Context, workspaceID, userID uuid.UUID, sessionID *uuid.UUID, callKind string, usage *dto.AIUsage) error {
	if usage == nil {
		return nil
	}

	currency := strings.TrimSpace(usage.Currency)
	if currency == "" {
		currency = "USD"
	}

	record := models.AIUsageRecord{
		ID:           uuid.New(),
		WorkspaceID:  workspaceID,
		UserID:       userID,
		SessionID:    sessionID,
		CallKind:     callKind,
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		TotalTokens:  usage.TotalTokens,
		Cost:         usage.Cost,
		Currency:     currency,
		CreatedAt:    time.Now(),
	}

	return q.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return fmt.Errorf("failed to write usage record: %w", err)
		}

		// Upsert aggregate — insert on first call, increment on subsequent ones.
		agg := models.WorkspaceQuotaAggregate{
			WorkspaceID: workspaceID,
			TotalTokens: usage.TotalTokens,
			TotalCost:   usage.Cost,
			Currency:    currency,
			UpdatedAt:   time.Now(),
		}
		result := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "workspace_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"total_tokens": gorm.Expr("workspace_quota_aggregates.total_tokens + ?", usage.TotalTokens),
				"total_cost":   gorm.Expr("workspace_quota_aggregates.total_cost + ?", usage.Cost),
				"updated_at":   time.Now(),
			}),
		}).Create(&agg)
		if result.Error != nil {
			return fmt.Errorf("failed to update quota aggregate: %w", result.Error)
		}
		return nil
	})
}

// GetAggregate returns the current quota aggregate for a workspace.
// Returns a zero-value aggregate (not an error) if no usage has been
// recorded yet.
func (q *QuotaService) GetAggregate(ctx context.Context, workspaceID uuid.UUID) (models.WorkspaceQuotaAggregate, error) {
	var agg models.WorkspaceQuotaAggregate
	err := q.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		First(&agg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.WorkspaceQuotaAggregate{WorkspaceID: workspaceID}, nil
	}
	return agg, err
}

func quotaLimitFromEnv() int64 {
	raw := strings.TrimSpace(os.Getenv(aiQuotaLimitTokensEnv))
	if raw == "" {
		return defaultQuotaLimitTokens
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return defaultQuotaLimitTokens
	}
	return v
}

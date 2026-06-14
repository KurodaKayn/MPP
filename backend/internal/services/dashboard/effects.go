package dashboard

import (
	"context"

	"github.com/google/uuid"
)

type dashboardProjectListInvalidator interface {
	InvalidateDashboardProjectListCache(ctx context.Context)
}

type dashboardStatsInvalidator interface {
	InvalidateDashboardStatsCache(ctx context.Context)
}

type dashboardReadModelUpdater interface {
	RefreshProjectAsync(ctx context.Context, projectID uuid.UUID)
	RefreshWorkspaceAsync(ctx context.Context, workspaceID uuid.UUID)
}

type dashboardSideEffects struct {
	projectLists dashboardProjectListInvalidator
	stats        dashboardStatsInvalidator
	readModels   dashboardReadModelUpdater
}

func (e dashboardSideEffects) InvalidateDashboardProjectListCache(ctx context.Context) {
	if e.projectLists == nil {
		return
	}
	e.projectLists.InvalidateDashboardProjectListCache(ctx)
}

func (e dashboardSideEffects) InvalidateDashboardStatsCache(ctx context.Context) {
	if e.stats == nil {
		return
	}
	e.stats.InvalidateDashboardStatsCache(ctx)
}

func (e dashboardSideEffects) RefreshProjectAsync(ctx context.Context, projectID uuid.UUID) {
	if e.readModels == nil {
		return
	}
	e.readModels.RefreshProjectAsync(ctx, projectID)
}

func (e dashboardSideEffects) RefreshWorkspaceAsync(ctx context.Context, workspaceID uuid.UUID) {
	if e.readModels == nil {
		return
	}
	e.readModels.RefreshWorkspaceAsync(ctx, workspaceID)
}

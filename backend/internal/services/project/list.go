package project

import (
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	projectpresenter "github.com/kurodakayn/mpp-backend/internal/services/project/presenter"
)

func (s *Service) ListProjects(page, limit int, status, filterUserID, platform string, scopeUserID *uuid.UUID) (*dto.PaginationResponse, error) {
	return s.ListProjectsCursor("", page, limit, status, filterUserID, platform, scopeUserID)
}

func (s *Service) ListProjectsCursor(cursor string, page, limit int, status, filterUserID, platform string, scopeUserID *uuid.UUID) (*dto.PaginationResponse, error) {
	if s.canUseDashboardProjectListCache() {
		return s.getCachedDashboardProjectList(cursor, page, limit, status, filterUserID, platform, scopeUserID)
	}
	return s.computeProjectList(cursor, page, limit, status, filterUserID, platform, scopeUserID)
}

func (s *Service) CanUseDashboardProjectListCache() bool {
	return s.canUseDashboardProjectListCache()
}

func (s *Service) ListCachedWorkspaceProjects(workspaceID uuid.UUID, actorUserID uuid.UUID, cursor string, page, limit int, status, platform string) (*dto.PaginationResponse, error) {
	return s.getCachedWorkspaceProjectList(workspaceID, actorUserID, cursor, page, limit, status, platform)
}

func (s *Service) computeProjectList(cursor string, page, limit int, status, filterUserID, platform string, scopeUserID *uuid.UUID) (*dto.PaginationResponse, error) {
	if scopeUserID == nil && platform == "" && s.canUseReadModels() {
		if resp, ok, err := s.adminProjectListFromReadModel(cursor, page, limit, status, filterUserID); err != nil {
			return nil, err
		} else if ok {
			return resp, nil
		}
	}

	query := s.projectListReadDB(scopeUserID).Model(&models.Project{})
	if scopeUserID != nil {
		query = s.ScopeAccessibleProjects(query, *scopeUserID)
	} else if filterUserID != "" {
		if uid, err := uuid.Parse(filterUserID); err == nil {
			query = query.Where("user_id = ?", uid)
		}
	}

	if status != "" {
		query = query.Where("status = ?", status)
	}
	if platform != "" {
		query = query.Joins("JOIN project_platform_publications ppp ON ppp.project_id = projects.id").
			Where("ppp.platform = ?", platform).
			Group("projects.id")
	}

	return s.ListProjectPage(query, cursor, page, limit, scopeUserID)
}

func (s *Service) computeWorkspaceProjectList(workspaceID uuid.UUID, actorUserID uuid.UUID, cursor string, page, limit int, status, platform string) (*dto.PaginationResponse, error) {
	query := s.strongReadDB().Model(&models.Project{}).Where("workspace_id = ?", workspaceID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if platform != "" {
		query = query.Joins("JOIN project_platform_publications ppp ON ppp.project_id = projects.id").
			Where("ppp.platform = ?", platform).
			Group("projects.id")
	}

	return s.ListProjectPage(query, cursor, page, limit, &actorUserID)
}

func (s *Service) adminProjectListFromReadModel(cursor string, page, limit int, status, filterUserID string) (*dto.PaginationResponse, bool, error) {
	query := s.projectListReadDB(nil).Model(&models.ProjectListSummary{})
	factQuery := s.projectListReadDB(nil).Model(&models.Project{})
	if filterUserID != "" {
		if uid, err := uuid.Parse(filterUserID); err == nil {
			query = query.Where("user_id = ?", uid)
			factQuery = factQuery.Where("user_id = ?", uid)
		}
	}
	if status != "" {
		query = query.Where("status = ?", status)
		factQuery = factQuery.Where("status = ?", status)
	}

	var summaryCount int64
	if err := query.Count(&summaryCount).Error; err != nil {
		return nil, false, err
	}
	if summaryCount == 0 {
		return nil, false, nil
	}
	var factCount int64
	if err := factQuery.Count(&factCount).Error; err != nil {
		return nil, false, err
	}
	if summaryCount != factCount {
		return nil, false, nil
	}

	resp, err := s.ListProjectSummaryPage(query, cursor, page, limit)
	if err != nil {
		return nil, false, err
	}
	return resp, true, nil
}

func (s *Service) projectListReadDB(scopeUserID *uuid.UUID) *gorm.DB {
	if scopeUserID != nil {
		return s.strongReadDB()
	}
	return s.eventualReadDB()
}

func (s *Service) ListProjectPage(query *gorm.DB, cursor string, page, limit int, scopeUserID *uuid.UUID) (*dto.PaginationResponse, error) {
	var projects []models.Project

	page, limit = normalizeProjectListPage(page, limit)
	query, err := applyProjectListCursor(query, cursor)
	if err != nil {
		return nil, err
	}

	if err := query.Omit("source_content").
		Preload("Publications", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, project_id, platform, enabled, status, draft_status, review_status, sync_required, publish_url")
		}).
		Order("projects.created_at DESC").
		Order("projects.id ASC").
		Limit(limit + 1).
		Find(&projects).Error; err != nil {
		return nil, err
	}

	hasMore := len(projects) > limit
	if hasMore {
		projects = projects[:limit]
	}

	accessByProjectID := make(map[uuid.UUID]projectAccessResolution, len(projects))
	if scopeUserID != nil {
		var accessErr error
		accessByProjectID, accessErr = s.projectAccessForUser(projects, *scopeUserID)
		if accessErr != nil {
			return nil, accessErr
		}
	} else {
		for _, project := range projects {
			accessByProjectID[project.ID] = projectAccessResolution{
				role:   models.ProjectRoleOwner,
				source: models.ProjectAccessSourceOwner,
			}
		}
	}

	items := make([]dto.ProjectListItem, 0, len(projects))
	for _, p := range projects {
		access := accessByProjectID[p.ID]
		items = append(items, projectpresenter.ProjectListItemFromModel(p, access.role, access.source))
	}

	nextCursor := ""
	if hasMore && len(projects) > 0 {
		nextCursor = encodeProjectListCursor(projects[len(projects)-1])
	}

	return projectPaginationResponse(items, cursor, page, limit, hasMore, nextCursor), nil
}

func (s *Service) ListProjectSummaryPage(query *gorm.DB, cursor string, page, limit int) (*dto.PaginationResponse, error) {
	var summaries []models.ProjectListSummary

	page, limit = normalizeProjectListPage(page, limit)
	query, err := applyProjectListCursorColumns(query, cursor, "project_list_summaries.created_at", "project_list_summaries.project_id")
	if err != nil {
		return nil, err
	}

	if err := query.
		Order("project_list_summaries.created_at DESC").
		Order("project_list_summaries.project_id ASC").
		Limit(limit + 1).
		Find(&summaries).Error; err != nil {
		return nil, err
	}

	hasMore := len(summaries) > limit
	if hasMore {
		summaries = summaries[:limit]
	}

	items := make([]dto.ProjectListItem, 0, len(summaries))
	for _, summary := range summaries {
		item, err := projectpresenter.ProjectListItemFromSummary(summary, models.ProjectRoleOwner, models.ProjectAccessSourceOwner)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	nextCursor := ""
	if hasMore && len(summaries) > 0 {
		last := summaries[len(summaries)-1]
		nextCursor = encodeProjectListCursorValues(last.CreatedAt, last.ProjectID)
	}

	return projectPaginationResponse(items, cursor, page, limit, hasMore, nextCursor), nil
}

func projectPaginationResponse(items []dto.ProjectListItem, cursor string, page int, limit int, hasMore bool, nextCursor string) *dto.PaginationResponse {
	total := int64((page-1)*limit + len(items))
	if hasMore {
		total++
	}
	totalPages := page
	if len(items) == 0 && page == 1 {
		totalPages = 0
	} else if hasMore {
		totalPages = page + 1
	}

	return &dto.PaginationResponse{
		Items:      items,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
		Cursor:     strings.TrimSpace(cursor),
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}
}

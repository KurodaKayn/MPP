package workspace

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
)

const (
	defaultWorkspaceActivityLimit = 20
	maxWorkspaceActivityLimit     = 100
)

func normalizeWorkspaceName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrInvalidWorkspace
	}
	return name, nil
}

func normalizeWorkspaceSlug(slug string) string {
	return strings.TrimSpace(slug)
}

func normalizeWorkspaceMemberRole(role string) (string, error) {
	role = strings.TrimSpace(role)
	switch role {
	case models.WorkspaceRoleAdmin, models.WorkspaceRoleMember, models.WorkspaceRoleViewer:
		return role, nil
	default:
		return "", ErrInvalidWorkspaceMember
	}
}

func canManageWorkspaceRole(role string) bool {
	return RoleHasPermission(role, PermissionManageMembers)
}

func canCreateWorkspaceProjectRole(role string) bool {
	return RoleHasPermission(role, PermissionProjectCreate)
}

func normalizeWorkspaceActivityLimit(limit int) int {
	if limit < 1 {
		return defaultWorkspaceActivityLimit
	}
	if limit > maxWorkspaceActivityLimit {
		return maxWorkspaceActivityLimit
	}
	return limit
}

func workspaceActivityMetadata(metadata map[string]any) (datatypes.JSON, error) {
	if metadata == nil {
		return datatypes.JSON([]byte(`{}`)), nil
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(payload), nil
}

func (s *Service) recordWorkspaceActivity(tx *gorm.DB, workspaceID uuid.UUID, actorUserID uuid.UUID, eventType string, targetUserID *uuid.UUID, metadata map[string]any) error {
	payload, err := workspaceActivityMetadata(metadata)
	if err != nil {
		return err
	}

	return tx.Create(&models.WorkspaceActivity{
		WorkspaceID:  workspaceID,
		ActorUserID:  actorUserID,
		TargetUserID: targetUserID,
		EventType:    eventType,
		Metadata:     payload,
	}).Error
}

func (s *Service) workspaceAccessRole(workspaceID uuid.UUID, userID uuid.UUID) (string, error) {
	if workspaceID == uuid.Nil || userID == uuid.Nil {
		return "", ErrInvalidWorkspace
	}

	var workspace models.Workspace
	if err := s.db.Select("id", "owner_user_id").First(&workspace, "id = ?", workspaceID).Error; err != nil {
		return "", err
	}
	if workspace.OwnerUserID == userID {
		return models.WorkspaceRoleOwner, nil
	}

	var member models.WorkspaceMember
	if err := s.db.
		Select("workspace_id", "user_id", "role").
		Where("workspace_id = ? AND user_id = ?", workspaceID, userID).
		First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrForbidden
		}
		return "", err
	}
	return member.Role, nil
}

func (s *Service) requireWorkspaceManager(workspaceID uuid.UUID, userID uuid.UUID) (string, error) {
	return s.RequirePermission(workspaceID, userID, PermissionManageMembers)
}

func (s *Service) EnsurePersonalWorkspace(userID uuid.UUID) (*dto.Workspace, error) {
	if userID == uuid.Nil {
		return nil, ErrInvalidWorkspace
	}

	workspaceID := models.PersonalWorkspaceID(userID)
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := ensurePersonalWorkspaceWithDB(tx, userID); err != nil {
			return err
		}
		return tx.Model(&models.Project{}).
			Where("user_id = ? AND (workspace_id IS NULL OR workspace_id = ?)", userID, uuid.Nil).
			Update("workspace_id", workspaceID).Error
	}); err != nil {
		return nil, err
	}
	return s.GetWorkspace(workspaceID, userID)
}

func ensurePersonalWorkspaceWithDB(tx *gorm.DB, ownerUserID uuid.UUID) error {
	workspaceID := models.PersonalWorkspaceID(ownerUserID)
	workspace := models.Workspace{
		ID:          workspaceID,
		OwnerUserID: ownerUserID,
		Name:        models.PersonalWorkspaceName,
		Slug:        models.PersonalWorkspaceSlug(ownerUserID),
		Status:      models.WorkspaceStatusActive,
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoNothing: true,
	}).Create(&workspace).Error; err != nil {
		return err
	}

	now := time.Now().UTC()
	member := models.WorkspaceMember{
		WorkspaceID: workspaceID,
		UserID:      ownerUserID,
		Role:        models.WorkspaceRoleOwner,
		JoinedAt:    &now,
	}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "workspace_id"}, {Name: "user_id"}},
		DoNothing: true,
	}).Create(&member).Error
}

func (s *Service) ListWorkspaces(userID uuid.UUID) (*dto.WorkspacesResponse, error) {
	if userID == uuid.Nil {
		return nil, ErrInvalidWorkspace
	}
	if _, err := s.EnsurePersonalWorkspace(userID); err != nil {
		return nil, err
	}

	memberWorkspaceIDs := s.db.
		Model(&models.WorkspaceMember{}).
		Select("workspace_id").
		Where("user_id = ?", userID)

	var workspaces []models.Workspace
	if err := s.db.
		Where("owner_user_id = ? OR id IN (?)", userID, memberWorkspaceIDs).
		Order("updated_at DESC").
		Order("id ASC").
		Find(&workspaces).Error; err != nil {
		return nil, err
	}

	roles, err := s.workspaceRolesForUser(workspaces, userID)
	if err != nil {
		return nil, err
	}

	items := make([]dto.Workspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		items = append(items, workspaceFromModel(workspace, roles[workspace.ID]))
	}
	return &dto.WorkspacesResponse{Items: items}, nil
}

func (s *Service) ListWorkspaceProjects(workspaceID uuid.UUID, actorUserID uuid.UUID, page, limit int, status, platform string) (*dto.PaginationResponse, error) {
	if _, err := s.workspaceAccessRole(workspaceID, actorUserID); err != nil {
		return nil, err
	}

	query := s.db.Model(&models.Project{}).Where("workspace_id = ?", workspaceID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if platform != "" {
		query = query.Joins("JOIN project_platform_publications ppp ON ppp.project_id = projects.id").
			Where("ppp.platform = ?", platform).
			Group("projects.id")
	}

	return s.projects.ListProjectPage(query, page, limit, &actorUserID)
}

func (s *Service) CreateWorkspaceProject(workspaceID uuid.UUID, actorUserID uuid.UUID, req dto.CreateProjectRequest) (*dto.ProjectListItem, error) {
	if _, err := s.RequirePermission(workspaceID, actorUserID, PermissionProjectCreate); err != nil {
		return nil, err
	}

	return s.projects.CreateProjectWithWorkspace(actorUserID, &workspaceID, req)
}

func (s *Service) CreateWorkspace(actorUserID uuid.UUID, req dto.CreateWorkspaceRequest) (*dto.Workspace, error) {
	if actorUserID == uuid.Nil {
		return nil, ErrInvalidWorkspace
	}
	name, err := normalizeWorkspaceName(req.Name)
	if err != nil {
		return nil, err
	}

	var workspace models.Workspace
	err = s.db.Transaction(func(tx *gorm.DB) error {
		workspace = models.Workspace{
			OwnerUserID: actorUserID,
			Name:        name,
			Slug:        normalizeWorkspaceSlug(req.Slug),
			Status:      models.WorkspaceStatusActive,
		}
		if err := tx.Create(&workspace).Error; err != nil {
			return err
		}

		now := time.Now()
		member := models.WorkspaceMember{
			WorkspaceID: workspace.ID,
			UserID:      actorUserID,
			Role:        models.WorkspaceRoleOwner,
			JoinedAt:    &now,
		}
		if err := tx.Create(&member).Error; err != nil {
			return err
		}
		return s.recordWorkspaceActivity(tx, workspace.ID, actorUserID, models.WorkspaceActivityWorkspaceCreated, nil, map[string]any{
			"name": workspace.Name,
			"slug": workspace.Slug,
		})
	})
	if err != nil {
		return nil, err
	}

	item := workspaceFromModel(workspace, models.WorkspaceRoleOwner)
	return &item, nil
}

func (s *Service) GetWorkspace(workspaceID uuid.UUID, actorUserID uuid.UUID) (*dto.Workspace, error) {
	role, err := s.workspaceAccessRole(workspaceID, actorUserID)
	if err != nil {
		return nil, err
	}

	var workspace models.Workspace
	if err := s.db.First(&workspace, "id = ?", workspaceID).Error; err != nil {
		return nil, err
	}
	item := workspaceFromModel(workspace, role)
	return &item, nil
}

func (s *Service) UpdateWorkspace(workspaceID uuid.UUID, actorUserID uuid.UUID, req dto.UpdateWorkspaceRequest) (*dto.Workspace, error) {
	if _, err := s.requireWorkspaceManager(workspaceID, actorUserID); err != nil {
		return nil, err
	}
	name, err := normalizeWorkspaceName(req.Name)
	if err != nil {
		return nil, err
	}

	nextSlug := normalizeWorkspaceSlug(req.Slug)
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var workspace models.Workspace
		if err := tx.Select("id", "name", "slug").First(&workspace, "id = ?", workspaceID).Error; err != nil {
			return err
		}
		if workspace.Name == name && workspace.Slug == nextSlug {
			return nil
		}
		updates := map[string]any{
			"name": name,
			"slug": nextSlug,
		}
		if err := tx.Model(&workspace).Updates(updates).Error; err != nil {
			return err
		}
		return s.recordWorkspaceActivity(tx, workspaceID, actorUserID, models.WorkspaceActivityWorkspaceUpdated, nil, map[string]any{
			"previous_name": workspace.Name,
			"previous_slug": workspace.Slug,
			"name":          name,
			"slug":          nextSlug,
		})
	}); err != nil {
		return nil, err
	}
	return s.GetWorkspace(workspaceID, actorUserID)
}

func (s *Service) ListWorkspaceMembers(workspaceID uuid.UUID, actorUserID uuid.UUID) (*dto.WorkspaceMembersResponse, error) {
	if _, err := s.requireWorkspaceManager(workspaceID, actorUserID); err != nil {
		return nil, err
	}

	var members []models.WorkspaceMember
	if err := s.db.
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "username", "email")
		}).
		Where("workspace_id = ?", workspaceID).
		Order("created_at ASC").
		Order("CASE role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 WHEN 'member' THEN 2 ELSE 3 END ASC").
		Order("user_id ASC").
		Find(&members).Error; err != nil {
		return nil, err
	}

	items := make([]dto.WorkspaceMember, 0, len(members))
	for _, member := range members {
		items = append(items, workspaceMemberFromModel(member))
	}
	return &dto.WorkspaceMembersResponse{Items: items}, nil
}

func (s *Service) ListWorkspaceActivities(workspaceID uuid.UUID, actorUserID uuid.UUID, limit int) (*dto.WorkspaceActivitiesResponse, error) {
	if _, err := s.requireWorkspaceManager(workspaceID, actorUserID); err != nil {
		return nil, err
	}

	var activities []models.WorkspaceActivity
	if err := s.db.
		Preload("Actor", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "username", "email")
		}).
		Preload("TargetUser", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "username", "email")
		}).
		Where("workspace_id = ?", workspaceID).
		Order("created_at DESC").
		Order("id DESC").
		Limit(normalizeWorkspaceActivityLimit(limit)).
		Find(&activities).Error; err != nil {
		return nil, err
	}

	items := make([]dto.WorkspaceActivity, 0, len(activities))
	for _, activity := range activities {
		items = append(items, workspaceActivityFromModel(activity))
	}
	return &dto.WorkspaceActivitiesResponse{Items: items}, nil
}

func (s *Service) AddWorkspaceMember(workspaceID uuid.UUID, actorUserID uuid.UUID, req dto.AddWorkspaceMemberRequest) (*dto.WorkspaceMember, error) {
	if _, err := s.requireWorkspaceManager(workspaceID, actorUserID); err != nil {
		return nil, err
	}
	role, err := normalizeWorkspaceMemberRole(req.Role)
	if err != nil {
		return nil, err
	}

	user, err := s.resolveWorkspaceMemberUser(req)
	if err != nil {
		return nil, err
	}

	var workspace models.Workspace
	if err := s.db.Select("id", "owner_user_id").First(&workspace, "id = ?", workspaceID).Error; err != nil {
		return nil, err
	}
	if user.ID == workspace.OwnerUserID {
		return nil, ErrInvalidWorkspaceMember
	}

	now := time.Now()
	targetUserID := user.ID
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		eventType := models.WorkspaceActivityMemberAdded
		metadata := map[string]any{
			"role": role,
		}
		shouldRecordActivity := true

		var existing models.WorkspaceMember
		if err := tx.Select("role").First(&existing, "workspace_id = ? AND user_id = ?", workspaceID, user.ID).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		} else if existing.Role == role {
			shouldRecordActivity = false
		} else {
			eventType = models.WorkspaceActivityMemberRoleChanged
			metadata["previous_role"] = existing.Role
		}

		member := models.WorkspaceMember{
			WorkspaceID: workspaceID,
			UserID:      user.ID,
			Role:        role,
			InvitedBy:   &actorUserID,
			JoinedAt:    &now,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "workspace_id"},
				{Name: "user_id"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"role":       role,
				"invited_by": actorUserID,
				"joined_at":  now,
			}),
		}).Create(&member).Error; err != nil {
			return err
		}

		if !shouldRecordActivity {
			return nil
		}
		return s.recordWorkspaceActivity(tx, workspaceID, actorUserID, eventType, &targetUserID, metadata)
	}); err != nil {
		return nil, err
	}

	return s.getWorkspaceMember(workspaceID, user.ID)
}

func (s *Service) CreateWorkspaceInvite(workspaceID uuid.UUID, actorUserID uuid.UUID, req dto.CreateWorkspaceInviteRequest) (*dto.WorkspaceInviteWithToken, error) {
	if _, err := s.RequirePermission(workspaceID, actorUserID, PermissionManageMembers); err != nil {
		return nil, err
	}
	role, err := normalizeWorkspaceMemberRole(req.Role)
	if err != nil {
		return nil, err
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || !strings.Contains(email, "@") {
		return nil, ErrInvalidWorkspaceInvite
	}
	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour)
	if req.ExpiresAt != nil {
		expiresAt = req.ExpiresAt.UTC()
	}
	if !expiresAt.After(time.Now().UTC()) {
		return nil, ErrInvalidWorkspaceInvite
	}
	token, tokenHash, err := newWorkspaceInviteToken()
	if err != nil {
		return nil, err
	}

	invite := models.WorkspaceInvite{
		WorkspaceID: workspaceID,
		Email:       email,
		Role:        role,
		InvitedBy:   actorUserID,
		Status:      models.WorkspaceInviteStatusPending,
		TokenHash:   tokenHash,
		ExpiresAt:   expiresAt,
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&invite).Error; err != nil {
			return err
		}
		return s.recordWorkspaceActivity(tx, workspaceID, actorUserID, models.WorkspaceActivityInviteCreated, nil, map[string]any{
			"email": email,
			"role":  role,
		})
	}); err != nil {
		return nil, err
	}

	item := workspaceInviteFromModel(invite)
	return &dto.WorkspaceInviteWithToken{WorkspaceInvite: item, Token: token}, nil
}

func (s *Service) ListWorkspaceInvites(workspaceID uuid.UUID, actorUserID uuid.UUID) (*dto.WorkspaceInvitesResponse, error) {
	if _, err := s.RequirePermission(workspaceID, actorUserID, PermissionManageMembers); err != nil {
		return nil, err
	}
	if err := s.expireWorkspaceInvites(workspaceID); err != nil {
		return nil, err
	}

	var invites []models.WorkspaceInvite
	if err := s.db.
		Where("workspace_id = ?", workspaceID).
		Order("created_at DESC").
		Find(&invites).Error; err != nil {
		return nil, err
	}
	items := make([]dto.WorkspaceInvite, 0, len(invites))
	for _, invite := range invites {
		items = append(items, workspaceInviteFromModel(invite))
	}
	return &dto.WorkspaceInvitesResponse{Items: items}, nil
}

func (s *Service) AcceptWorkspaceInvite(actorUserID uuid.UUID, req dto.AcceptWorkspaceInviteRequest) (*dto.WorkspaceMember, error) {
	if actorUserID == uuid.Nil {
		return nil, ErrInvalidWorkspaceInvite
	}
	tokenHash := hashWorkspaceInviteToken(strings.TrimSpace(req.Token))
	if tokenHash == "" {
		return nil, ErrInvalidWorkspaceInvite
	}

	var member models.WorkspaceMember
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var invite models.WorkspaceInvite
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&invite, "token_hash = ?", tokenHash).Error; err != nil {
			return err
		}
		if invite.Status != models.WorkspaceInviteStatusPending {
			return ErrInvalidWorkspaceInvite
		}
		now := time.Now().UTC()
		if !invite.ExpiresAt.After(now) {
			return tx.Model(&invite).Update("status", models.WorkspaceInviteStatusExpired).Error
		}
		var user models.User
		if err := tx.Select("id", "email").First(&user, "id = ?", actorUserID).Error; err != nil {
			return err
		}
		if !strings.EqualFold(user.Email, invite.Email) {
			return ErrForbidden
		}
		member = models.WorkspaceMember{
			WorkspaceID: invite.WorkspaceID,
			UserID:      actorUserID,
			Role:        invite.Role,
			InvitedBy:   &invite.InvitedBy,
			JoinedAt:    &now,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "workspace_id"}, {Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"role":       invite.Role,
				"invited_by": invite.InvitedBy,
				"joined_at":  now,
			}),
		}).Create(&member).Error; err != nil {
			return err
		}
		if err := tx.Model(&invite).Updates(map[string]any{
			"accepted_by": actorUserID,
			"accepted_at": &now,
			"status":      models.WorkspaceInviteStatusAccepted,
		}).Error; err != nil {
			return err
		}
		return s.recordWorkspaceActivity(tx, invite.WorkspaceID, actorUserID, models.WorkspaceActivityInviteAccepted, &actorUserID, map[string]any{
			"email": invite.Email,
			"role":  invite.Role,
		})
	}); err != nil {
		return nil, err
	}
	return s.getWorkspaceMember(member.WorkspaceID, actorUserID)
}

func (s *Service) RevokeWorkspaceInvite(workspaceID uuid.UUID, actorUserID uuid.UUID, inviteID uuid.UUID) error {
	if _, err := s.RequirePermission(workspaceID, actorUserID, PermissionManageMembers); err != nil {
		return err
	}
	if inviteID == uuid.Nil {
		return ErrInvalidWorkspaceInvite
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var invite models.WorkspaceInvite
		if err := tx.First(&invite, "id = ? AND workspace_id = ?", inviteID, workspaceID).Error; err != nil {
			return err
		}
		if invite.Status != models.WorkspaceInviteStatusPending {
			return ErrInvalidWorkspaceInvite
		}
		now := time.Now().UTC()
		if err := tx.Model(&invite).Updates(map[string]any{
			"status":     models.WorkspaceInviteStatusRevoked,
			"revoked_at": &now,
		}).Error; err != nil {
			return err
		}
		return s.recordWorkspaceActivity(tx, workspaceID, actorUserID, models.WorkspaceActivityInviteRevoked, nil, map[string]any{
			"email": invite.Email,
			"role":  invite.Role,
		})
	})
}

func (s *Service) UpdateWorkspaceMember(workspaceID uuid.UUID, actorUserID uuid.UUID, targetUserID uuid.UUID, req dto.UpdateWorkspaceMemberRequest) (*dto.WorkspaceMember, error) {
	if _, err := s.requireWorkspaceManager(workspaceID, actorUserID); err != nil {
		return nil, err
	}
	if targetUserID == uuid.Nil {
		return nil, ErrInvalidWorkspaceMember
	}
	role, err := normalizeWorkspaceMemberRole(req.Role)
	if err != nil {
		return nil, err
	}

	var workspace models.Workspace
	if err := s.db.Select("id", "owner_user_id").First(&workspace, "id = ?", workspaceID).Error; err != nil {
		return nil, err
	}
	if targetUserID == workspace.OwnerUserID {
		return nil, ErrInvalidWorkspaceMember
	}

	var member models.WorkspaceMember
	if err := s.db.Where("workspace_id = ? AND user_id = ?", workspaceID, targetUserID).First(&member).Error; err != nil {
		return nil, err
	}
	if member.Role == role {
		return s.getWorkspaceMember(workspaceID, targetUserID)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&member).Update("role", role).Error; err != nil {
			return err
		}
		return s.recordWorkspaceActivity(tx, workspaceID, actorUserID, models.WorkspaceActivityMemberRoleChanged, &targetUserID, map[string]any{
			"previous_role": member.Role,
			"role":          role,
		})
	}); err != nil {
		return nil, err
	}
	return s.getWorkspaceMember(workspaceID, targetUserID)
}

func (s *Service) RemoveWorkspaceMember(workspaceID uuid.UUID, actorUserID uuid.UUID, targetUserID uuid.UUID) error {
	if _, err := s.requireWorkspaceManager(workspaceID, actorUserID); err != nil {
		return err
	}
	if targetUserID == uuid.Nil {
		return ErrInvalidWorkspaceMember
	}

	var workspace models.Workspace
	if err := s.db.Select("id", "owner_user_id").First(&workspace, "id = ?", workspaceID).Error; err != nil {
		return err
	}
	if targetUserID == workspace.OwnerUserID {
		return ErrInvalidWorkspaceMember
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		var member models.WorkspaceMember
		if err := tx.First(&member, "workspace_id = ? AND user_id = ?", workspaceID, targetUserID).Error; err != nil {
			return err
		}
		result := tx.Delete(&models.WorkspaceMember{}, "workspace_id = ? AND user_id = ?", workspaceID, targetUserID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return s.recordWorkspaceActivity(tx, workspaceID, actorUserID, models.WorkspaceActivityMemberRemoved, &targetUserID, map[string]any{
			"previous_role": member.Role,
		})
	})
}

func (s *Service) resolveWorkspaceMemberUser(req dto.AddWorkspaceMemberRequest) (*models.User, error) {
	var user models.User
	if req.UserID != uuid.Nil {
		if err := s.db.Select("id", "username", "email").First(&user, "id = ?", req.UserID).Error; err != nil {
			return nil, err
		}
		return &user, nil
	}

	email := strings.TrimSpace(req.Email)
	if email == "" {
		return nil, ErrInvalidWorkspaceMember
	}
	if err := s.db.
		Select("id", "username", "email").
		Where("LOWER(email) = LOWER(?)", email).
		First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) getWorkspaceMember(workspaceID uuid.UUID, userID uuid.UUID) (*dto.WorkspaceMember, error) {
	var member models.WorkspaceMember
	if err := s.db.
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "username", "email")
		}).
		Where("workspace_id = ? AND user_id = ?", workspaceID, userID).
		First(&member).Error; err != nil {
		return nil, err
	}
	item := workspaceMemberFromModel(member)
	return &item, nil
}

func (s *Service) workspaceRolesForUser(workspaces []models.Workspace, userID uuid.UUID) (map[uuid.UUID]string, error) {
	roles := make(map[uuid.UUID]string, len(workspaces))
	memberWorkspaceIDs := make([]uuid.UUID, 0)
	for _, workspace := range workspaces {
		if workspace.OwnerUserID == userID {
			roles[workspace.ID] = models.WorkspaceRoleOwner
			continue
		}
		memberWorkspaceIDs = append(memberWorkspaceIDs, workspace.ID)
	}
	if len(memberWorkspaceIDs) == 0 {
		return roles, nil
	}

	var members []models.WorkspaceMember
	if err := s.db.
		Select("workspace_id", "role").
		Where("user_id = ? AND workspace_id IN ?", userID, memberWorkspaceIDs).
		Find(&members).Error; err != nil {
		return nil, err
	}
	for _, member := range members {
		roles[member.WorkspaceID] = member.Role
	}
	return roles, nil
}

func workspaceFromModel(workspace models.Workspace, role string) dto.Workspace {
	return dto.Workspace{
		ID:          workspace.ID,
		OwnerUserID: workspace.OwnerUserID,
		Name:        workspace.Name,
		Slug:        workspace.Slug,
		Status:      workspace.Status,
		Role:        role,
		CreatedAt:   workspace.CreatedAt,
		UpdatedAt:   workspace.UpdatedAt,
	}
}

func workspaceMemberFromModel(member models.WorkspaceMember) dto.WorkspaceMember {
	return dto.WorkspaceMember{
		WorkspaceID: member.WorkspaceID,
		UserID:      member.UserID,
		Username:    member.User.Username,
		Email:       member.User.Email,
		Role:        member.Role,
		InvitedBy:   member.InvitedBy,
		JoinedAt:    member.JoinedAt,
		CreatedAt:   member.CreatedAt,
	}
}

func workspaceInviteFromModel(invite models.WorkspaceInvite) dto.WorkspaceInvite {
	return dto.WorkspaceInvite{
		ID:          invite.ID,
		WorkspaceID: invite.WorkspaceID,
		Email:       invite.Email,
		Role:        invite.Role,
		InvitedBy:   invite.InvitedBy,
		AcceptedBy:  invite.AcceptedBy,
		Status:      invite.Status,
		ExpiresAt:   invite.ExpiresAt,
		AcceptedAt:  invite.AcceptedAt,
		RevokedAt:   invite.RevokedAt,
		CreatedAt:   invite.CreatedAt,
		UpdatedAt:   invite.UpdatedAt,
	}
}

func newWorkspaceInviteToken() (string, string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", fmt.Errorf("create workspace invite token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	return token, hashWorkspaceInviteToken(token), nil
}

func hashWorkspaceInviteToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *Service) expireWorkspaceInvites(workspaceID uuid.UUID) error {
	return s.db.Model(&models.WorkspaceInvite{}).
		Where("workspace_id = ? AND status = ? AND expires_at <= ?", workspaceID, models.WorkspaceInviteStatusPending, time.Now().UTC()).
		Update("status", models.WorkspaceInviteStatusExpired).Error
}

func workspaceActivityFromModel(activity models.WorkspaceActivity) dto.WorkspaceActivity {
	metadata := map[string]any{}
	if len(activity.Metadata) > 0 {
		_ = json.Unmarshal(activity.Metadata, &metadata)
	}

	item := dto.WorkspaceActivity{
		ID:            activity.ID,
		WorkspaceID:   activity.WorkspaceID,
		ActorUserID:   activity.ActorUserID,
		ActorUsername: activity.Actor.Username,
		ActorEmail:    activity.Actor.Email,
		TargetUserID:  activity.TargetUserID,
		EventType:     activity.EventType,
		Metadata:      metadata,
		CreatedAt:     activity.CreatedAt,
	}
	if activity.TargetUser != nil {
		item.TargetUsername = activity.TargetUser.Username
		item.TargetEmail = activity.TargetUser.Email
	}
	return item
}

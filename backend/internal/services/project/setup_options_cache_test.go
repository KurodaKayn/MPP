package project_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestContentSetupOptionsCacheCachesPersonalLists(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient, redisServer := newContentSetupRedisClientWithServer(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	user := createContentSetupCacheUser(t, db, "setup-personal")
	workspaceID := models.PersonalWorkspaceID(user.ID)
	template := seedContentSetupTemplate(t, db, "personal-cached", models.ContentTemplateScopePersonal, user.ID, uuid.Nil)
	profile := seedContentSetupBrandProfile(t, db, workspaceID, user.ID, "personal-brand-cached")

	requireContentSetupCacheKeys(t, redisClient, "content-templates", 0)
	requireContentSetupCacheKeys(t, redisClient, "brand-profiles", 0)

	templates, err := s.WithContext(context.Background()).ListContentTemplates(user.ID, uuid.Nil)
	require.NoError(t, err)
	require.Len(t, templates.Items, 1)
	require.Equal(t, template.ID, templates.Items[0].ID)
	templateKey := requireSingleContentSetupCacheKey(t, redisClient, "content-templates")
	require.Contains(t, templateKey, "content-templates")
	require.Contains(t, templateKey, "user:"+user.ID.String())
	require.Contains(t, templateKey, "workspace:"+workspaceID.String())

	profiles, err := s.WithContext(context.Background()).ListBrandProfiles(user.ID, uuid.Nil)
	require.NoError(t, err)
	require.Len(t, profiles.Items, 1)
	require.Equal(t, profile.ID, profiles.Items[0].ID)
	profileKey := requireSingleContentSetupCacheKey(t, redisClient, "brand-profiles")
	require.Contains(t, profileKey, "brand-profiles")
	require.Contains(t, profileKey, "user:"+user.ID.String())
	require.Contains(t, profileKey, "workspace:"+workspaceID.String())

	seedContentSetupTemplate(t, db, "personal-fresh", models.ContentTemplateScopePersonal, user.ID, uuid.Nil)
	seedContentSetupBrandProfile(t, db, workspaceID, user.ID, "personal-brand-fresh")

	cachedTemplates, err := s.WithContext(context.Background()).ListContentTemplates(user.ID, uuid.Nil)
	require.NoError(t, err)
	require.Len(t, cachedTemplates.Items, 1)
	require.Equal(t, template.ID, cachedTemplates.Items[0].ID)
	cachedProfiles, err := s.WithContext(context.Background()).ListBrandProfiles(user.ID, uuid.Nil)
	require.NoError(t, err)
	require.Len(t, cachedProfiles.Items, 1)
	require.Equal(t, profile.ID, cachedProfiles.Items[0].ID)

	redisServer.FastForward(16 * time.Second)

	refreshedTemplates, err := s.WithContext(context.Background()).ListContentTemplates(user.ID, uuid.Nil)
	require.NoError(t, err)
	require.Len(t, refreshedTemplates.Items, 2)
	refreshedProfiles, err := s.WithContext(context.Background()).ListBrandProfiles(user.ID, uuid.Nil)
	require.NoError(t, err)
	require.Len(t, refreshedProfiles.Items, 2)
}

func TestContentSetupOptionsCacheCachesWorkspaceLists(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newContentSetupRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	owner := createContentSetupCacheUser(t, db, "setup-workspace")
	workspace := createContentSetupCacheWorkspace(t, db, owner, "Setup Workspace")
	seedContentSetupTemplate(t, db, "workspace-personal", models.ContentTemplateScopePersonal, owner.ID, uuid.Nil)
	workspaceTemplate := seedContentSetupTemplate(t, db, "workspace-cached", models.ContentTemplateScopeWorkspace, uuid.Nil, workspace.ID)
	brandProfile := seedContentSetupBrandProfile(t, db, workspace.ID, owner.ID, "workspace-brand-cached")

	templates, err := s.WithContext(context.Background()).ListContentTemplates(owner.ID, workspace.ID)
	require.NoError(t, err)
	require.Len(t, templates.Items, 2)
	requireContentSetupItemIDs(t, templates.Items, workspaceTemplate.ID)
	templateKey := requireSingleContentSetupCacheKey(t, redisClient, "content-templates")
	require.Contains(t, templateKey, "user:"+owner.ID.String())
	require.Contains(t, templateKey, "workspace:"+workspace.ID.String())

	profiles, err := s.WithContext(context.Background()).ListBrandProfiles(owner.ID, workspace.ID)
	require.NoError(t, err)
	require.Len(t, profiles.Items, 1)
	require.Equal(t, brandProfile.ID, profiles.Items[0].ID)
	profileKey := requireSingleContentSetupCacheKey(t, redisClient, "brand-profiles")
	require.Contains(t, profileKey, "user:"+owner.ID.String())
	require.Contains(t, profileKey, "workspace:"+workspace.ID.String())

	seedContentSetupTemplate(t, db, "workspace-fresh", models.ContentTemplateScopeWorkspace, uuid.Nil, workspace.ID)
	seedContentSetupBrandProfile(t, db, workspace.ID, owner.ID, "workspace-brand-fresh")

	cachedTemplates, err := s.WithContext(context.Background()).ListContentTemplates(owner.ID, workspace.ID)
	require.NoError(t, err)
	require.Len(t, cachedTemplates.Items, 2)
	cachedProfiles, err := s.WithContext(context.Background()).ListBrandProfiles(owner.ID, workspace.ID)
	require.NoError(t, err)
	require.Len(t, cachedProfiles.Items, 1)
	require.Equal(t, brandProfile.ID, cachedProfiles.Items[0].ID)
}

func TestCreateContentTemplateInvalidatesOnlyTemplateCache(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newContentSetupRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	owner := createContentSetupCacheUser(t, db, "setup-template-invalidate")
	workspace := createContentSetupCacheWorkspace(t, db, owner, "Template Invalidate Workspace")
	seedContentSetupTemplate(t, db, "template-cached", models.ContentTemplateScopeWorkspace, uuid.Nil, workspace.ID)
	seedContentSetupBrandProfile(t, db, workspace.ID, owner.ID, "brand-cached")

	_, err := s.WithContext(context.Background()).ListContentTemplates(owner.ID, workspace.ID)
	require.NoError(t, err)
	templateKey := requireSingleContentSetupCacheKey(t, redisClient, "content-templates")
	_, err = s.WithContext(context.Background()).ListBrandProfiles(owner.ID, workspace.ID)
	require.NoError(t, err)
	profileKey := requireSingleContentSetupCacheKey(t, redisClient, "brand-profiles")

	_, err = s.WithContext(context.Background()).CreateContentTemplate(owner.ID, workspace.ID, dto.CreateContentTemplateRequest{
		Name:             "template-created",
		TitleTemplate:    "Created title",
		SourceTemplate:   "Created body",
		DefaultPlatforms: []string{"wechat"},
	})
	require.NoError(t, err)

	requireContentSetupCacheKeys(t, redisClient, "content-templates", 0)
	require.Contains(t, requireContentSetupCacheKeys(t, redisClient, "brand-profiles", 1), profileKey)
	require.NotContains(t, requireContentSetupCacheKeys(t, redisClient, "brand-profiles", 1), templateKey)

	refreshed, err := s.WithContext(context.Background()).ListContentTemplates(owner.ID, workspace.ID)
	require.NoError(t, err)
	require.Len(t, refreshed.Items, 2)
}

func TestCreateBrandProfileInvalidatesOnlyBrandProfileCache(t *testing.T) {
	db := testsupport.SetupTestDB()
	redisClient := newContentSetupRedisClient(t)
	s := services.NewDashboardService(db)
	s.UseRedis(redisClient)

	owner := createContentSetupCacheUser(t, db, "setup-brand-invalidate")
	workspace := createContentSetupCacheWorkspace(t, db, owner, "Brand Invalidate Workspace")
	seedContentSetupTemplate(t, db, "template-cached", models.ContentTemplateScopeWorkspace, uuid.Nil, workspace.ID)
	seedContentSetupBrandProfile(t, db, workspace.ID, owner.ID, "brand-cached")

	_, err := s.WithContext(context.Background()).ListContentTemplates(owner.ID, workspace.ID)
	require.NoError(t, err)
	templateKey := requireSingleContentSetupCacheKey(t, redisClient, "content-templates")
	_, err = s.WithContext(context.Background()).ListBrandProfiles(owner.ID, workspace.ID)
	require.NoError(t, err)

	_, err = s.WithContext(context.Background()).CreateBrandProfile(owner.ID, workspace.ID, dto.CreateBrandProfileRequest{
		Name: "brand-created",
	})
	require.NoError(t, err)

	requireContentSetupCacheKeys(t, redisClient, "brand-profiles", 0)
	require.Contains(t, requireContentSetupCacheKeys(t, redisClient, "content-templates", 1), templateKey)

	refreshed, err := s.WithContext(context.Background()).ListBrandProfiles(owner.ID, workspace.ID)
	require.NoError(t, err)
	require.Len(t, refreshed.Items, 2)
}

func newContentSetupRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	client, _ := newContentSetupRedisClientWithServer(t)
	return client
}

func newContentSetupRedisClientWithServer(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()

	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	return client, redisServer
}

func createContentSetupCacheUser(t *testing.T, db *gorm.DB, prefix string) models.User {
	t.Helper()

	user := models.User{
		Username:     prefix + "-user",
		Email:        prefix + "@example.com",
		PasswordHash: "hash",
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func createContentSetupCacheWorkspace(t *testing.T, db *gorm.DB, owner models.User, name string) models.Workspace {
	t.Helper()

	workspace := models.Workspace{
		OwnerUserID: owner.ID,
		Name:        name,
		Status:      models.WorkspaceStatusActive,
	}
	require.NoError(t, db.Create(&workspace).Error)
	return workspace
}

func seedContentSetupTemplate(t *testing.T, db *gorm.DB, name string, scope string, ownerUserID uuid.UUID, workspaceID uuid.UUID) models.ContentTemplate {
	t.Helper()

	var ownerID *uuid.UUID
	if ownerUserID != uuid.Nil {
		ownerID = &ownerUserID
	}
	var scopedWorkspaceID *uuid.UUID
	if workspaceID != uuid.Nil {
		scopedWorkspaceID = &workspaceID
	}
	template := models.ContentTemplate{
		WorkspaceID:      scopedWorkspaceID,
		OwnerUserID:      ownerID,
		Scope:            scope,
		Name:             name,
		TitleTemplate:    "Title " + name,
		SourceTemplate:   "Body " + name,
		DefaultPlatforms: datatypes.JSON([]byte(`["wechat"]`)),
		PlatformConfig:   datatypes.JSON([]byte(`{}`)),
		Tags:             datatypes.JSON([]byte(`[]`)),
	}
	require.NoError(t, db.Create(&template).Error)
	return template
}

func seedContentSetupBrandProfile(t *testing.T, db *gorm.DB, workspaceID uuid.UUID, createdBy uuid.UUID, name string) models.BrandProfile {
	t.Helper()

	profile := models.BrandProfile{
		WorkspaceID: workspaceID,
		CreatedBy:   createdBy,
		Name:        name,
	}
	require.NoError(t, db.Create(&profile).Error)
	return profile
}

func requireSingleContentSetupCacheKey(t *testing.T, client *redis.Client, resource string) string {
	t.Helper()

	return requireContentSetupCacheKeys(t, client, resource, 1)[0]
}

func requireContentSetupCacheKeys(t *testing.T, client *redis.Client, resource string, count int) []string {
	t.Helper()

	cacheKeys, err := client.Keys(context.Background(), "mpp:dashboard:content-setup:v1:"+resource+":*").Result()
	require.NoError(t, err)
	sort.Strings(cacheKeys)
	require.Len(t, cacheKeys, count)
	return cacheKeys
}

func requireContentSetupItemIDs(t *testing.T, items []dto.ContentTemplate, ids ...uuid.UUID) {
	t.Helper()

	seen := make(map[uuid.UUID]struct{}, len(items))
	for _, item := range items {
		seen[item.ID] = struct{}{}
	}
	for _, id := range ids {
		require.Contains(t, seen, id)
	}
}

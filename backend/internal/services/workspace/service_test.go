package workspace

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

func TestRoleHasPermission(t *testing.T) {
	require.True(t, RoleHasPermission(models.WorkspaceRoleOwner, PermissionManageBilling))
	require.True(t, RoleHasPermission(models.WorkspaceRoleAdmin, PermissionManageMembers))
	require.True(t, RoleHasPermission(models.WorkspaceRoleMember, PermissionPublishSchedule))
	require.True(t, RoleHasPermission(models.WorkspaceRoleViewer, PermissionProjectReview))

	require.False(t, RoleHasPermission(models.WorkspaceRoleMember, PermissionManageMembers))
	require.False(t, RoleHasPermission(models.WorkspaceRoleViewer, PermissionPublishSchedule))
	require.False(t, RoleHasPermission(models.WorkspaceRoleViewer, PermissionPublishPublish))
	require.False(t, RoleHasPermission("unknown", PermissionProjectReview))
}

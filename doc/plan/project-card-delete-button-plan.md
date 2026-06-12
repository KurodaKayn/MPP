# Project Card Delete Button Plan

## 1. Goal

Add a real project deletion flow and replace the current top-right `ready` status badge on project cards with a trash icon button in both project card surfaces:

- `frontend/src/app/[locale]/dashboard/posts/_components/posts-page-content.tsx`
- `frontend/src/app/[locale]/dashboard/collab/_components/collaboration-hub-page.tsx`

The user-facing result is that project cards no longer show `ready` in the top-right action slot. Instead, deletable cards show a compact trash icon button. Deleting a project removes it from the relevant list without a full page reload.

## 2. Current Code State

### Frontend

- The posts page loads workspace projects with `getWorkspaceProjects()` and renders `PostProjectCard`.
- The collaboration hub loads three project groups:
  - `sharedByMeProjects`
  - `sharedWithMeProjects`
  - `workspaceProjects`
- Both `PostProjectCard` and collaboration `ProjectCard` render:
  - title
  - updated timestamp via `formatOptionalDashboardDate`
  - top-right `ProjectStatusBadge`
  - platform/publication metadata
  - a bottom open/edit button
- The UI already has an established delete icon pattern in `workspace-members-card.tsx`:
  - `Trash2` from `lucide-react`
  - `Button variant="ghost" size="icon-sm"`
  - destructive icon color
  - loading state with `Loader2`
  - accessible `sr-only` text
- `frontend/src/lib/dashboard/api/projects.ts` already has `fetchDashboardNoContent()` DELETE wrappers for collaborators, share links, schedules, and media, but no project delete wrapper.

### Backend

- User dashboard routes support project list/create/get/update/content/platform/publish/collab actions, but no `DELETE /api/user/dashboard/projects/:id`.
- `project.Service` supports create, get, update, content save, platform save, list, collaborator management, and project collaboration state sync.
- `DashboardService` embeds `project.Service`, so adding `DeleteProject()` to the project service makes it available to handlers through `h.serviceFor(c).DeleteProject(...)`.
- Project-related models include direct and indirect dependent rows:
  - `project_platform_publications`
  - `project_collaborators`
  - `project_activities`
  - `project_comments`
  - `project_versions`
  - `project_share_links`
  - `project_list_summaries`
  - `scheduled_publications`
  - `publish_attempts`
  - `publish_events`
  - `extension_callback_tokens`
  - `extension_execution_events`
  - `media_assets` and `media_asset_usages`
  - `platform_account_grants`
- Several associations declare `OnDelete:CASCADE` or `OnDelete:SET NULL`, but not every table has an explicit GORM foreign-key constraint. The delete service should not rely only on database cascades.

## 3. Recommended Approach

Implement real deletion end-to-end.

Do not only hide the card client-side. The current product has creation and editing but lacks a persisted delete action. A UI-only deletion would make the card reappear on refresh and would make the new trash button misleading.

Use this route:

```text
DELETE /api/user/dashboard/projects/:id
```

Return `204 No Content` on success.

## 4. Deletion Permission Model

Use a conservative permission model:

- Project owner can delete the project.
- Workspace owner/admin can delete a workspace project.
- Workspace member/editor can edit content but should not delete the whole project.
- Direct collaborators and shared-with-me users cannot delete the owner project.

Frontend visibility should mirror this as closely as possible:

- Posts page: use `workspaceSelection.selectedWorkspace.role` plus `project.role` to decide whether to enable the trash button.
- Collaboration hub:
  - `sharedByMeProjects`: deletable because these are owner projects.
  - `workspaceProjects`: deletable only for selected workspace owner/admin, or if `project.role === "owner"`.
  - `sharedWithMeProjects`: do not allow deletion; replace the status badge slot with a disabled trash button so `ready` is still gone and the layout stays stable.

Backend remains the source of truth and must return `403 Forbidden` if the frontend gets this wrong.

## 5. Backend Work Plan

### Step 1: Add Project Delete Service

Add `DeleteProject(projectID uuid.UUID, userID uuid.UUID) error` in `backend/internal/services/project/lifecycle.go` or a new focused `delete.go`.

Flow:

1. Validate non-empty IDs.
2. Load the project with `id`, `user_id`, `workspace_id`, and `status`.
3. Authorize deletion:
   - allow if `project.UserID == userID`
   - otherwise, if the project has a workspace, allow workspace owner/admin
   - otherwise return `ErrForbidden`
4. Block deletion when there is active publishing work:
   - publication status `queued` or `publishing`
   - scheduled publication status `running`
   - scheduled publication status `needs_manual_action`, because the project may still require user completion on an external platform
5. In a transaction, clean dependent data explicitly:
   - delete publish attempts for schedules belonging to the project
   - delete scheduled publications for the project
   - delete publish events for the project
   - delete extension callback tokens and execution events for the project
   - delete project media usages
   - set `media_assets.project_id = NULL` for project-scoped assets instead of deleting stored objects
   - delete platform account grants scoped to the project
   - delete project list summary
   - delete project platform publications
   - delete collaborators, activities, comments, versions, share links
   - delete the project row
6. Invalidate dashboard project list cache and stats cache.
7. Delete/update the read model summary through the existing read model updater path when available.

Add a project-specific error, for example `ErrProjectDeletionBlocked`, and expose it through `backend/internal/services/dashboard/facade.go`.

### Step 2: Add Handler and Route

Add `DeleteProject(c echo.Context) error` to `backend/internal/handlers/user_dashboard.go`.

Behavior:

- invalid UUID -> `400 invalid_request`
- not found -> `404 not_found`
- forbidden -> `403 forbidden`
- active publish/schedule guard -> `409 conflict`
- success -> `204 No Content`

Register it in `backend/cmd/api/server.go`:

```go
userGroup.DELETE("/projects/:id", h.userDashboard.DeleteProject)
```

### Step 3: Update Contracts If Required

If route contracts are tracked for endpoint definitions, add the DELETE operation to `contracts/openapi.yaml` or the relevant project component path and regenerate:

- backend generated contracts if this repo uses them for route/schema checks
- frontend generated API schema if route-level generation is expected

The current frontend API client is manually wrapped, so the minimum frontend change does not require a generated response type.

## 6. Frontend Work Plan

### Step 1: Add API Wrapper

Add to `frontend/src/lib/dashboard/api/projects.ts`:

```ts
export function deleteDashboardProject(projectId: string) {
  return fetchDashboardNoContent(
    `/api/user/dashboard/projects/${projectId}`,
    { method: "DELETE" },
  );
}
```

Add or update API tests in `frontend/src/lib/dashboard/api.test.ts` to assert the method and path.

### Step 2: Add Shared Delete Button Behavior

Avoid duplicating deletion state across both pages. Prefer a small reusable helper/component near dashboard components, for example:

- `frontend/src/app/[locale]/dashboard/_components/project-delete-button.tsx`

Responsibilities:

- render `Button variant="ghost" size="icon-sm"`
- show `Trash2` normally and `Loader2` while deleting
- expose `sr-only` text from translation keys
- accept `disabled`, `isDeleting`, and `onDelete`
- optionally call `window.confirm(...)` before deleting if no existing AlertDialog component is available

Use existing UI patterns rather than introducing a new dialog system unless the project already has a confirmation component ready to reuse.

### Step 3: Posts Page Integration

In `posts-page-content.tsx`:

1. Import `deleteDashboardProject`, `Trash2`/shared button, `Loader2`, and `toast` if needed.
2. Track `deletingProjectId`.
3. Add `handleDeleteProject(project)`:
   - confirm destructive action
   - call `deleteDashboardProject(project.id)`
   - remove the deleted project from `projects`
   - show success toast
   - on failure, show error toast and keep the project in the list
4. Pass delete state and handler to `PostProjectCard`.
5. Replace the top-right `ProjectStatusBadge` with the trash icon button.
6. Keep card layout stable: top row remains `justify-between`; the action button has fixed icon-button dimensions.

### Step 4: Collaboration Hub Integration

In `collaboration-hub-page.tsx`:

1. Track `deletingProjectId`.
2. Add one `handleDeleteProject(project)` shared by the three sections.
3. On successful deletion, remove the project from:
   - `allProjects`
   - `sharedByMeProjects`
   - `workspaceProjects`
4. Do not remove unrelated shared-with-me projects unless the deleted ID matches.
5. Replace the top-right `ProjectStatusBadge` with the delete action slot.
6. For non-deletable `sharedWithMeProjects`, do not show `ready`; render a disabled trash button with accessible text explaining no permission.

### Step 5: Translation Keys

Add dashboard namespace strings for both Chinese and English locales, following the existing translation structure:

- delete button accessible label
- confirm text
- success toast
- failure toast
- no-permission label if a disabled action is used

Suggested keys:

- `project.delete.label`
- `project.delete.confirm`
- `project.delete.success`
- `project.delete.failed`
- `project.delete.noPermission`

## 7. Testing Plan

### Backend Tests

Add service tests in `backend/internal/services/project/lifecycle_test.go` or a new delete test file:

- owner can delete project and dependent rows are removed or detached as designed
- workspace owner/admin can delete a workspace project
- workspace member/editor cannot delete if deletion is restricted to owner/admin
- direct collaborator/viewer cannot delete
- deletion is blocked when publication is queued/publishing
- cache/read model invalidation happens enough that lists no longer return the deleted project

Add handler tests in `backend/internal/handlers/dashboard_test.go`:

- invalid UUID -> 400
- missing project -> 404
- forbidden -> 403
- active publish guard -> 409
- success -> 204

Add route registration coverage if route tests enumerate registered dashboard routes.

### Frontend Tests

Add API test:

- `deleteDashboardProject("project-1")` calls `DELETE /api/user/dashboard/projects/project-1`

Add component/page tests where existing test harnesses make this practical:

- posts page removes a project card after delete succeeds
- posts page keeps the card and shows failure toast when delete fails
- collaboration hub removes the project from relevant local lists
- shared-with-me cards do not show the `ready` badge

### Manual Verification

After implementation:

1. Start the app with the normal dev command.
2. Open the posts page and verify ready badge is gone.
3. Delete a project and refresh; project remains gone.
4. Open collaboration hub and verify the same behavior in shared-by-me and workspace sections.
5. Verify shared-with-me projects cannot delete someone else's project.
6. Verify active publishing projects produce a clear failure instead of disappearing locally.

## 8. Implementation Order

1. Create a feature branch, for example `feat/project-card-delete-button`.
2. Add backend service deletion and tests.
3. Add handler route and handler tests.
4. Add frontend API wrapper and API test.
5. Add reusable delete button and translations.
6. Wire posts page.
7. Wire collaboration hub.
8. Run formatters and focused tests.
9. Run broader backend and frontend test commands if time permits.
10. Do a browser pass for layout and interaction.

## 9. Acceptance Criteria

- The top-right `ready` badge is gone from project cards in posts and collaboration hub.
- Deletable cards show a trash icon button in that position.
- The delete action persists through backend deletion.
- The deleted card is removed from the UI immediately after success.
- Failed deletion leaves the card visible and shows an error.
- Users without delete permission cannot delete shared projects.
- Backend tests cover permission and dependency cleanup.
- Frontend API/UI tests cover successful and failed delete paths.

## 10. Open Decisions

- Whether to hard-delete all project history or keep archived activity/audit rows. The current request sounds like deletion, so the first implementation can hard-delete project-owned rows, but this is a product decision.
- Whether workspace members with editor-like access should be allowed to delete. This plan recommends owner/admin only.
- Whether project media files should be deleted from object storage. This plan recommends detaching media assets first, because media library scopes can be workspace/personal and object deletion may surprise users.

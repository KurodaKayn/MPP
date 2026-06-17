# MPP SaaS Feature Design Plan

## 1. Goals and Boundaries

Goal: evolve MPP from a single-user publishing tool into a team-oriented, multi-platform content publishing workspace.

Scope includes only SaaS product and business design:

- Workspaces, members, roles, invitations, permissions, and auditing.
- Platform account connections, shared authorization, health status, and re-login flows.
- Content projects, collaborative editing, templates, brand presets, media library, comments, and versions.
- Platform drafts, target accounts, scheduled publishing, publishing calendar, and result tracking.
- Publishing approvals, tasks, and notifications.
- Plans, quotas, usage, billing status, and admin management.

This plan does not cover high concurrency, distributed deployment, queues, container scheduling, gateways, or database scaling plans.

## 2. Current Business Foundation

Already available:

- `Workspace`, `WorkspaceMember`, and `WorkspaceActivity`, supporting workspaces, members, and activity records.
- `Project` as the main content object, already linked to `workspace_id`, collaborative documents, and platform publication records.
- Project collaborators, share links, comments, version records, and project activities.
- `MediaAsset`, already carrying `workspace_id`, `project_id`, and the object-storage boundary.
- `PlatformAccount`, currently storing account data, credentials, cookies, status, and test results by user and platform.
- `RemoteBrowserSession`, supporting platform login-state capture and currently managing sessions by user and platform.
- `ProjectPlatformPublication`, storing platform drafts, publishing status, remote IDs, publish URLs, errors, and retry counts.
- `PublishEvent`, recording publishing execution events.

Main gaps:

- Workspace is not yet an explicit SaaS tenant, plan, usage, and billing boundary.
- Platform accounts are still closer to personal assets and lack workspace-level sharing, account aliases, multi-account selection, and authorization visibility.
- Publishing permissions are still owner-oriented and lack publisher permissions, approval boundaries, and target-account authorization.
- Content production has project collaboration, but lacks templates, brand presets, media library, and pre-publish checks.
- Publishing already has status and events, but lacks a scheduling model, publishing calendar, manual-action states, and version freezing.
- Plans, quotas, usage events, billing status, and quota interception points are missing.
- Auditing and notifications are scattered and lack a unified event stream and task entry point.

## 3. Core Design

### 3.1 Workspace

Workspace is the SaaS tenant, collaboration, resource ownership, and billing boundary.

Field direction:

- `workspaces`: add `plan_code`, `billing_status`, `billing_customer_ref`, `trial_ends_at`, `default_timezone`, and `settings`.
- `workspace_members`: keep `owner`, `admin`, `member`, and `viewer`.
- `workspace_activities`: expand event types to cover invitations, accounts, scheduling, publishing, approvals, quotas, and billing status changes.

New objects:

- `WorkspaceInvite`: email, role, inviter, expiration time, and accepted state.
- `WorkspaceSeat`: member seat state, join time, and release time.

Rules:

- Personal workspaces are created automatically; team workspaces support inviting members, managing accounts, and managing plans.
- Every dashboard query carries the current `workspace_id`.
- New business resources must carry `workspace_id` by default.
- After switching workspaces, content, accounts, media, templates, schedules, statistics, and billing are all filtered by the current workspace.

### 3.2 Members and Permissions

Keep roles minimal and permission points explicit. The service layer should check permission points instead of scattering role-name checks.

| Permission | owner | admin | member | viewer |
| ---------- | ----- | ----- | ------ | ------ |
| Manage billing | Yes | Configurable | No | No |
| Manage members | Yes | Yes | No | No |
| Manage platform accounts | Yes | Yes | No | No |
| Use authorized accounts | Yes | Yes | Yes | No |
| Create projects | Yes | Yes | Yes | No |
| Edit projects | Yes | Yes | Yes | No |
| Comment and review | Yes | Yes | Yes | Yes |
| Start publishing | Yes | Yes | Yes | No |
| Approve publishing | Yes | Yes | Configurable | No |
| View publishing calendar | Yes | Yes | Yes | Yes |

Key permission points:

- `workspace.manage_billing`
- `workspace.manage_members`
- `account.connect`
- `account.manage`
- `account.use`
- `project.create`
- `project.edit`
- `project.review`
- `publication.approve`
- `publication.publish`
- `publication.schedule`

Rules:

- Keep project-level collaborators for overriding default workspace permissions.
- Compute workspace permissions from the base role first, then overlay direct project grants.
- Publishing permission must satisfy project edit permission, account-use permission, approval status, and remaining quota at the same time.
- The frontend hides entries without permission, while the backend still performs strict validation.

### 3.3 Platform Account Assets

Platform accounts evolve from personal connection records into workspace-authorizable assets.

Field direction:

- `platform_accounts`: add `workspace_id`, `owner_user_id`, `connected_by_user_id`, `display_name`, `platform_user_id`, `share_scope`, `last_connected_at`, `last_verified_at`, `expires_at`, `health_status`, and `credential_secret_ref`.
- Adjust the unique constraint from `user_id + platform` to `workspace_id + platform + platform_user_id`; when the remote ID is missing, use `workspace_id + platform + display_name`.
- `remote_browser_sessions`: add `workspace_id` and `platform_account_id`.

New objects:

- `PlatformAccountGrant`: grants an account to a member or project, with roles `manager`, `publisher`, and `viewer`.
- `PlatformAccountHealthCheck`: latest check result, error category, and next check time.

Rules:

- The account connection entry point lives in the current workspace.
- Users can choose "only available to me" or "available to the workspace".
- After cookie capture, credentials are written to secret storage; business tables only keep `credential_secret_ref`.
- Publishing tasks reference only `platform_account_id` and do not copy cookies.
- When an account disconnects, cookies expire, credential tests fail, or a platform risk-control event occurs, mark it as `needs_reauth`.
- Before removing an account, show affected schedules and drafts.

### 3.4 Content Projects

`Project` remains the main content-production object; do not add a duplicate main content table.

Field direction:

- `Project.workspace_id` is required.
- `Project.template_id` is optional.
- `Project.brand_profile_id` is optional.
- `Project.status`: `draft`, `reviewing`, `approved`, `scheduled`, `publishing`, `published`, `failed`.

Rules:

- Project lists support filtering by status, platform, author, update time, and scheduling status.
- Project details integrate source content, platform drafts, comments, versions, activities, and publishing results.
- Project permissions show their source: owner, workspace role, project collaborator, or share link.
- `ProjectVersion` is used as the basis for approval, rollback, and scheduled publishing.
- After restoring a version, prompt the user to resync platform drafts.

### 3.5 Templates, Brand, and Media Library

New objects:

- `ContentTemplate`: title rules, body structure, tag rules, default platforms, platform parameters, and applicable scope.
- `BrandProfile`: tone, address style, banned words, CTA, link strategy, and default tags.
- `ProjectChecklist`: pre-publish check items such as cover, title, platform account, approval, and media status.
- `MediaAssetVariant`: crop, compression, cover specs, and platform adaptation result.
- `MediaAssetUsage`: which project, publication, or template uses a media asset.

Field direction:

- `media_assets`: add `library_scope`, `tags`, `alt_text`, `source`, and `derivative_of`.

Rules:

- Templates have three levels: system, workspace, and personal.
- AI editing includes template and brand context, but only generates proposals; data is persisted only after the user accepts.
- Uploaded media first enters `pending`, then becomes `ready` after processing.
- Publishing can reference only `ready` media.
- Media deletion defaults to soft delete; physical deletion is forbidden when a media asset is referenced by published records.

### 3.6 Platform Drafts and Publishing Targets

Current `ProjectPlatformPublication` is `project + platform`; later it must express `project + platform + account`.

Field direction:

- Add `platform_account_id` to support multiple accounts on the same platform.
- `config` stores platform publishing parameters, such as collection, tags, visibility, and original-content declaration.
- `adapted_content` stores platform draft content.
- `status` continues to express draft, syncing, queued, publishing, succeeded, failed, and cancelled states.

Rules:

- The platform draft panel supports choosing a target account for each platform.
- The same project may target multiple accounts on the same platform; the UI defaults to one account per platform.
- After platform drafts change, mark them as needing sync or re-approval.
- Publishing results are shown by account, not only by platform.
- Failed results keep readable errors, failure time, retry entry point, and account-health entry point.

### 3.7 Scheduled Publishing

Scheduling is an independent business object and should not be squeezed into project status.

New objects:

- `ScheduledPublication`
  - `workspace_id`
  - `project_id`
  - `publication_id`
  - `platform_account_id`
  - `project_version_id`
  - `scheduled_at`
  - `timezone`
  - `status`: `draft`, `pending_review`, `approved`, `scheduled`, `running`, `published`, `failed`, `needs_manual_action`, `cancelled`
  - `idempotency_key`
  - `created_by`
  - `approved_by`
  - `cancelled_by`
  - `last_error`
- `PublishAttempt`
  - `scheduled_publication_id`
  - `attempt_no`
  - `started_at`
  - `finished_at`
  - `status`
  - `remote_id`
  - `publish_url`
  - `error_code`
  - `error_message`

Rules:

- Immediate publishing is also a `ScheduledPublication`, with `scheduled_at = now`.
- A schedule binds to `ProjectVersion`; after content changes, the user must reconfirm or re-approve it.
- Before a schedule is due, validate account state, account authorization, platform draft sync, permissions, and quota.
- If browser-based publishing encounters CAPTCHA, second confirmation, or expired login, transition to `needs_manual_action` and keep the remote session entry point until TTL.
- Failure can be retried, but retry must reuse the same `idempotency_key`.
- Cancellation, failure, and rescheduling all have independent records.

### 3.8 Approval

Approval only serves pre-publishing quality control and is not a complex workflow engine.

New objects:

- `ApprovalRequest`
  - `workspace_id`
  - `project_id`
  - `project_version_id`
  - `publication_id`
  - `platform_account_id`
  - `scheduled_publication_id`
  - `requested_by`
  - `status`: `pending`, `approved`, `rejected`, `cancelled`
  - `due_at`
- `ApprovalDecision`
  - `approval_request_id`
  - `reviewer_user_id`
  - `decision`
  - `comment`
  - `created_at`

Rules:

- Workspace settings support "approval required before publishing".
- When approval is required, members can submit publishing requests, and owners/admins or members with approval permission can approve them.
- Approval records bind to the project version, platform draft, target account, and schedule.
- If draft content, target account, or schedule content changes, any existing approval is automatically invalidated.
- Approval comments use the existing `ProjectComment`; they are not written into the body text.

### 3.9 Plans, Quotas, and Usage

Plans control product capabilities and resource limits, not deployment architecture.

New objects:

- `Plan`: member count, platform account count, monthly publish count, monthly schedule count, AI usage, browser minutes, media storage, and version-history retention time.
- `WorkspaceSubscription`: current plan, billing status, billing period, and external customer reference.
- `UsageEvent`: append-only usage event.
- `UsageAggregate`: aggregate count for the current period.
- `QuotaOverride`: temporary top-up or manual adjustment.

Usage event fields:

- `workspace_id`
- `actor_user_id`
- `event_type`
- `quantity`
- `unit`
- `resource_type`
- `resource_id`
- `idempotency_key`
- `created_at`

Quota interception points:

- Invite member.
- Connect platform account.
- Upload media.
- AI editing.
- Start browser session.
- Publish immediately.
- Schedule publishing.

Rules:

- Implement manual plans and quotas first so product validation is not blocked.
- Over-limit state blocks new resource creation or new task starts, but does not interrupt running tasks.
- Usage is first written to `UsageEvent`, then asynchronously aggregated into `UsageAggregate`.
- Every metering event carries `idempotency_key` to prevent duplicate deductions during retries.
- Over-limit responses use a unified error code, and the frontend shows upgrade or cleanup actions.

### 3.10 Audit, Notifications, and Admin

Add a unified `AuditEvent`:

- `workspace_id`
- `actor_user_id`
- `event_type`
- `resource_type`
- `resource_id`
- `metadata`
- `created_at`

Add a unified `Notification`:

- `workspace_id`
- `recipient_user_id`
- `event_type`
- `resource_type`
- `resource_id`
- `status`: `unread`, `read`, `archived`
- `created_at`

Key events:

- Member joined, removed, or role changed.
- Platform account connected, granted, re-login required, or deleted.
- Project created, version saved, approval accepted or rejected.
- Schedule created or cancelled, publishing started, publishing succeeded, publishing failed, or manual action required.
- Quota nearing limit, quota exceeded, or billing status changed.
- Admin changed plan, added quota, or froze publishing capability.

Rules:

- Start with in-app notifications; email and IM can come later.
- Approval tasks, accounts requiring re-login, publishing failures, manual-action items, and quota alerts must generate notifications.
- Audit events cannot be deleted by regular users.
- Admin can view audit activity but cannot view sensitive credentials.

## 4. Phase Plan

### Phase 1: Workspace SaaS Foundation

Goal: every business entry point has an explicit workspace context.

Status: Done.

Tasks:

- [x] Unify backend `workspace_id` context parsing and permission validation.
- [x] Make frontend dashboard routes and API requests carry the current workspace uniformly.
- [x] Filter projects, accounts, media, activities, and settings pages by the current workspace.
- [x] Automatically create personal workspaces and validate historical project backfill.
- [x] Add `WorkspaceInvite` with invitation, acceptance, expiration, and revocation.
- [x] Build role-to-permission mapping and complete service-layer guards.
- [x] Add unauthorized-access tests: cross-workspace reads, edits, publishing, and account usage must all be rejected.

Acceptance:

- [x] After switching workspaces, projects, accounts, media, and schedules from other workspaces are not visible.
- [x] Members without permission cannot modify members, accounts, publishing, or settings through the API.

### Phase 2: Workspace-scoped Platform Accounts

Goal: platform accounts become team-manageable, authorizable, and traceable resources.

Status: Done.

Tasks:

- [x] Adjust the account model to support `workspace_id`, `owner_user_id`, `connected_by_user_id`, account alias, and remote account ID.
- [x] Migrate old `user_id + platform` data to personal workspaces.
- [x] Create `PlatformAccountGrant` to support granting accounts to members or projects.
- [x] Bind remote browser login sessions to workspaces and platform accounts.
- [x] Change cookie/credential storage to secret refs, and do not return sensitive fields in business responses.
- [x] Account management page supports connect, test, share, rename, re-login, and remove.
- [x] A usable and authorized account must be selected before publishing.
- [x] Account anomalies are shown uniformly and block related publishing.

Acceptance:

- [x] One workspace can manage multiple accounts on the same platform.
- [x] Members can only use accounts they have permission for.
- [x] After an account becomes invalid, new publishing is blocked and a notification is produced.

### Phase 3: Collaborative Content Production Loop

Goal: teams can complete editing, template reuse, review, version recovery, and platform draft preparation around projects.

Status: Done.

Tasks:

- [x] Project details integrate source content, platform drafts, comments, versions, activities, and publishing results.
- [x] Add `ContentTemplate` with system, workspace, and personal scopes.
- [x] Add `BrandProfile`, bind it to workspace, and allow projects to reference it.
- [x] Add `template_id` and `brand_profile_id` to `Project`.
- [x] Complete `MediaAsset` library scope, tags, alt text, and source.
- [x] Add `MediaAssetUsage` to record media references.
- [x] Platform draft sync, editing, and review states are clear.
- [x] Prompt to resync platform drafts after version restore.
- [x] Show share-link and project-collaborator permissions merged with workspace permissions.

Acceptance:

- [x] Creating a project from a template can generate initial title, body structure, and default platforms.
- [x] A project can reference workspace media, and publishing can validate the media `ready` state.
- [x] An editor can edit content but cannot bypass publishing permissions.
- [x] A viewer can only view content, drafts, and results.

### Phase 4: Scheduled Publishing and Calendar

Goal: immediate publishing, scheduled publishing, failed retry, and manual handling all use one publishing-plan model.

Tasks:

- [x] Add `ScheduledPublication` and `PublishAttempt`.
- [x] Change immediate publishing to create `ScheduledPublication(scheduled_at=now)`.
- [x] Bind `ScheduledPublication` to `project_version_id`, `publication_id`, and `platform_account_id`.
- [x] Content page supports immediate publishing, scheduled publishing, and schedule cancellation.
- [ ] Publishing calendar supports day/week/month views.
- [ ] Before a schedule is due, validate account state, permissions, quota, and draft sync state.
- [ ] Add `needs_manual_action` state and record the remote session entry point and expiration time.
- [ ] Publishing details page shows attempts, failure reasons, retry entry point, and manual-action entry point.

Current Phase 4 implementation notes:

- `ScheduledPublication`/`PublishAttempt` data models, immediate-publish schedule records, scheduled publishing API, workspace publication calendar API, content-page schedule create/cancel, failed retry endpoint, and attempt display are in place.
- Pre-publish validation reuses the existing publishing path's project permission checks, account connection status, platform draft sync status, and media ready validation; the general publishing quota deduction model is not wired in yet.
- `needs_manual_action` status, remote session entry point, and expiration fields are in the model and DTO, and the content page shows the manual-action entry point; automatic transitions in the browser publishing flow still need to be refined by platform session result.
- The content page already shows attempts, failure reasons, retry entry point, and manual-action entry point; a standalone publishing details page is not done yet.
- Full day/week/month publishing calendar views and prompts to reconfirm/re-approve old schedules after content changes are still incomplete.

Acceptance:

- Immediate publishing and scheduled publishing use the same data model.
- After content changes, old schedules must prompt for reconfirmation or re-approval. Not done.
- Failed retry has complete attempt records and does not deduct publish quota twice.

### Phase 5: Approval Collaboration

Goal: teams have a clear and auditable approval flow before publishing.

Tasks:

- [ ] Add `ApprovalRequest` and `ApprovalDecision`.
- [ ] Add default approval policy to workspace settings.
- [ ] Allow projects to override approval policy.
- [ ] Freeze the current `ProjectVersion` when approval starts.
- [ ] Bind approval records to project, platform draft, target account, and schedule.
- [ ] After approval passes, invalidate approval if the project or platform draft changes.
- [ ] Wire approval validation into pre-publish checks.
- [ ] Add frontend approval tasks, approval details, approve/reject entry points.
- [ ] Write approval events to project activity, audit event, and notification.

Acceptance:

- Workspaces requiring approval cannot bypass approval when publishing.
- Approval records can be traced to a specific version, reviewer, time, and comment.
- Old approvals cannot continue to be used for publishing after content changes.

### Phase 6: Plans, Quotas, and Usage

Goal: SaaS commercial limits are enforceable, visible, and auditable.

Tasks:

- [ ] Add `Plan`, `WorkspaceSubscription`, `UsageEvent`, `UsageAggregate`, and `QuotaOverride`.
- [ ] Define metering metrics: seats, platform accounts, monthly publishes, monthly schedules, AI tokens/calls, browser minutes, and storage size.
- [ ] Wire quota interception points for members, accounts, media, AI, browser sessions, publishing, and scheduling.
- [ ] Aggregate current-period usage and provide a workspace billing API.
- [ ] Settings page shows plan, period usage, remaining quota, and upgrade entry point.
- [ ] Admin supports manually changing plan, adding quota, and disabling a workspace.
- [ ] Unify over-limit error codes and frontend prompts.
- [ ] Complete metering idempotency tests to ensure retries do not double-deduct.

Acceptance:

- Over-limit state cannot bypass the API.
- The usage page can explain which user consumed each quota, when, and why.
- Usage event and aggregate can be reconciled.

### Phase 7: Audit, Notifications, and Admin

Goal: member, account, approval, publishing, and billing events form a searchable and notifiable SaaS operations view.

Tasks:

- [ ] Add `AuditEvent` and wrap a unified recording function.
- [ ] Sync key workspace activity, project activity, and publish event records into audit events.
- [ ] Add `Notification` with in-app unread, read, and archive states.
- [ ] Approval tasks, account re-login, publishing failures, manual-action items, and quota alerts generate notifications.
- [ ] Add frontend notification center and audit log page.
- [ ] Admin can search users, workspaces, platform accounts, and projects.
- [ ] Admin can view audit activity but cannot view sensitive credentials.
- [ ] Admin can temporarily freeze publishing capability.
- [ ] All admin operations are written to an admin audit log.

Acceptance:

- Workspace owner/admin can query key operation audits.
- Users can see their pending approval, re-login, publishing failure, and manual-action tasks.
- Support/operations can locate workspace, account, schedule, and quota issues.
- Notification state changes do not affect audit records.

## 5. Priority List

| Priority | Task | Reason |
| -------- | ---- | ------ |
| P0 | Unified workspace context | All SaaS capabilities depend on workspace boundaries |
| P0 | Permission-point model | Avoid scattered role checks; publishing, approval, and account sharing all need it |
| P0 | Workspace-scoped platform accounts | Core of team collaboration for multi-platform publishing |
| P0 | Publisher permission | owner-only does not fit team publishing |
| P1 | Schedule object and publishing calendar | Core daily workflow for content teams |
| P1 | Approval switch | Quality control for team publishing |
| P1 | Audit and notifications | Accounts, approvals, publishing failures, and manual handling all need tasks |
| P1 | Plan and quota model | Commercialization and resource boundaries |
| P1 | Usage events | Quotas, billing, and auditing all depend on them |
| P2 | Templates, brand, and media library | Improves creation efficiency, but depends on a stable base content model |
| P2 | Admin management | Customer management and troubleshooting |
| P2 | Billing status linkage | Strengthen after charging starts |

## 6. Recommended Single-commit Boundaries

Recommended independent implementation commits:

1. Workspace context and permission guards.
2. Workspace invitations.
3. Platform account workspace migration.
4. Platform account grants.
5. Template, brand, and media library base models.
6. Scheduled publishing base model.
7. Publishing calendar API.
8. Approval request/decision.
9. Usage event/aggregate.
10. Billing settings API.
11. Audit events.
12. Notification center.
13. Admin workspace management.

## 7. Out of Scope

- Do not design service splitting, queues, gateways, Kubernetes, or database scaling.
- Do not split independent microservices by platform.
- Do not build a complex approval-flow engine.
- Do not build a public content community.
- Do not build a general project management tool.

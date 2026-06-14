import type { components } from "./generated";

type ContractSchema<Name extends keyof components["schemas"]> =
  components["schemas"][Name];

export type DashboardStats = ContractSchema<"DashboardStats">;
export type AdaptedContent = ContractSchema<"AdaptedContent">;
export type DraftFormat = ContractSchema<"DraftFormat">;
export type PublishPlatform = ContractSchema<"PublishPlatform">;
export type PublicationStatus = ContractSchema<"PublicationStatus">;
export type ProjectStatus = ContractSchema<"ProjectStatus">;
export type ProjectRole = ContractSchema<"ProjectRole">;
export type ProjectCollaboratorRole = ContractSchema<"ProjectCollaboratorRole">;
export type PublicationSummary = ContractSchema<"PublicationSummary">;
export type PublicationDetail = ContractSchema<"PublicationDetail">;
export type ProjectPublications = ContractSchema<"ProjectPublications">;
export type ContentTemplateScope = ContractSchema<"ContentTemplateScope">;
export type ContentTemplate = ContractSchema<"ContentTemplate">;
export type ContentTemplatesResponse =
  ContractSchema<"ContentTemplatesResponse">;
export type CreateContentTemplateInput =
  ContractSchema<"CreateContentTemplateRequest">;
export type BrandProfile = ContractSchema<"BrandProfile">;
export type BrandProfilesResponse = ContractSchema<"BrandProfilesResponse">;
export type CreateBrandProfileInput =
  ContractSchema<"CreateBrandProfileRequest">;
export type MediaAssetLibraryScope = ContractSchema<"MediaAssetLibraryScope">;
export type PublishResult = ContractSchema<"PublishResult">;
export type CollabDocumentRole = ContractSchema<"CollabDocumentRole">;
export type CollabDocumentSession = ContractSchema<"CollabDocumentSession">;
export type CollabDocument = ContractSchema<"CollabDocument">;
export type PaginatedCollabDocuments =
  ContractSchema<"PaginationCollabDocuments">;
export type CreateCollabDocumentInput =
  ContractSchema<"CreateCollabDocumentRequest">;
export type UpdateCollabDocumentInput =
  ContractSchema<"UpdateCollabDocumentRequest">;

export type PublishProjectOptions = {
  idempotencyKey?: string;
  mode?: "manual";
};

export type SchedulePublicationInput = {
  platform: PublishPlatform;
  scheduled_at: string;
  timezone?: string;
  idempotency_key?: string;
};

export type PublishAttempt = {
  id: string;
  scheduled_publication_id: string;
  attempt_no: number;
  started_at: string;
  finished_at?: string;
  status: string;
  remote_id?: string;
  publish_url?: string;
  error_code?: string;
  error_message?: string;
};

export type ScheduledPublication = {
  id: string;
  workspace_id: string;
  project_id: string;
  publication_id: string;
  platform_account_id?: string;
  project_version_id?: string;
  platform: PublishPlatform;
  project_title: string;
  scheduled_at: string;
  timezone: string;
  status: string;
  idempotency_key?: string;
  created_by: string;
  approved_by?: string;
  cancelled_by?: string;
  last_error?: string;
  manual_action_url?: string;
  manual_action_until?: string;
  attempts: PublishAttempt[];
  created_at: string;
  updated_at: string;
};

export type ScheduledPublicationsResponse = {
  items: ScheduledPublication[];
};

export type CreateProjectInput = {
  title: string;
  source_content: string;
  summary?: string;
  cover_image_url?: string;
  platforms: PublishPlatform[];
  template_id?: string;
  brand_profile_id?: string;
};

export type UpdateProjectInput = CreateProjectInput;

export type AddProjectCollaboratorInput =
  ContractSchema<"AddProjectCollaboratorRequest">;

export type UpdateProjectCollaboratorInput =
  ContractSchema<"UpdateProjectCollaboratorRequest">;

export type SaveProjectContentInput = Omit<CreateProjectInput, "platforms">;

export type SaveProjectPlatformsInput = {
  platforms: PublishPlatform[];
};

export type MediaUploadUsage = "cover_image" | "editor_image";

export type CreateMediaUploadInput = {
  filename: string;
  mime_type: string;
  size_bytes: number;
  usage: MediaUploadUsage;
  library_scope?: MediaAssetLibraryScope;
  tags?: string[];
  alt_text?: string;
  source?: string;
};

export type CreateMediaUploadResult = {
  asset_id: string;
  object_ref: string;
  upload_url: string;
  headers: Record<string, string>;
  expires_at: string;
};

export type CompleteMediaUploadResult = {
  asset_id: string;
  object_ref: string;
  status: string;
};

export type ResolvedMediaAsset = {
  asset_id: string;
  url: string;
  expires_at: string;
};

export type ResolveMediaAssetsResult = {
  items: ResolvedMediaAsset[];
};

export type GetProjectPublicationsOptions = {
  includeContent?: boolean;
};

export type FetchProjectPublications = (
  projectId: string,
  options?: GetProjectPublicationsOptions,
) => Promise<ProjectPublications>;

export type WaitForProjectPublicationsOptions = {
  intervalMs?: number;
  timeoutMs?: number;
  fetchProjectPublications?: FetchProjectPublications;
  sleep?: (ms: number) => Promise<void>;
};

export type SyncPrepublishInput = {
  platforms: PublishPlatform[];
  actor?: {
    type: "system";
  };
};

export type UpdatePrepublishDraftInput = {
  adapted_content: AdaptedContent;
};

export type AIChatMessage = ContractSchema<"AIChatMessage">;

export type AIEditContentStreamInput = {
  title?: string;
  content: string;
  message: string;
  conversation?: AIChatMessage[];
};

export type AIEditPrepublishStreamInput = {
  title?: string;
  platform: PublishPlatform;
  adapted_content: AdaptedContent;
  message: string;
  conversation?: AIChatMessage[];
};

export type AITextStreamOptions = {
  onChunk?: (chunk: string, accumulated: string) => void;
  signal?: AbortSignal;
};

export type AIGrowthOptimizationGoal =
  | "recommendation"
  | "views"
  | "ctr"
  | "completion"
  | "engagement"
  | "conversion";

export type AIGrowthOptimizationIntensity =
  | "conservative"
  | "balanced"
  | "aggressive";

export type AIGrowthOptimizationRunStatus =
  | "running"
  | "ready"
  | "applied"
  | "failed"
  | "cancelled";

export type AIProposalStatus =
  | "proposed"
  | "accepted"
  | "rejected"
  | "superseded";

export type AIQualityWarningSeverity = "info" | "warning" | "risk";

export type AIQualityWarning = {
  id: string;
  message: string;
  severity: AIQualityWarningSeverity;
};

export type AISourceProposal = {
  id: string;
  previous_content: string;
  previous_title: string;
  proposed_content: string;
  proposed_title: string;
  quality_warnings: AIQualityWarning[];
  status: AIProposalStatus;
  summary: string;
};

export type AIPlatformProposal = {
  id: string;
  previous_content: string;
  proposed_content: string;
  quality_warnings: AIQualityWarning[];
  status: AIProposalStatus;
  summary: string;
  target_platform: PublishPlatform;
};

export type AIGrowthOptimizationRun = {
  id: string;
  created_at: string;
  goal: AIGrowthOptimizationGoal;
  intensity: AIGrowthOptimizationIntensity;
  model: string;
  project_id: string;
  prompt_version: string;
  quality_warnings: AIQualityWarning[];
  source_proposal: AISourceProposal;
  status: AIGrowthOptimizationRunStatus;
  summary: string;
  target_platforms: PublishPlatform[];
  platform_proposals: AIPlatformProposal[];
  updated_at: string;
};

export type CreateAIGrowthOptimizationRunInput = {
  goal: AIGrowthOptimizationGoal;
  intensity: AIGrowthOptimizationIntensity;
  source_content: string;
  target_platforms: PublishPlatform[];
  title: string;
};

export type DecideAIProposalResult = {
  proposal_id: string;
  status: Extract<AIProposalStatus, "accepted" | "rejected">;
};
export type AIDraftingEventType = ContractSchema<"AIDraftingEventType">;
export type AIDraftingEvent = ContractSchema<"AIDraftingEvent">;
export type AIDraftingStreamEvent = Omit<
  Partial<AIDraftingEvent>,
  "event_type" | "payload"
> & {
  event_type: AIDraftingEventType | string;
  payload: Record<string, unknown>;
};

export type AIDraftingStreamOptions = {
  onEvent?: (event: AIDraftingStreamEvent) => void;
  signal?: AbortSignal;
};

export type StartAIDraftingSessionInput =
  ContractSchema<"AIDraftingStartRequest">;
export type ContinueAIDraftingSessionInput =
  ContractSchema<"AIDraftingContinueRequest">;
export type AIDraftingReplay = ContractSchema<"AIDraftingReplay">;

export type AIDraftingSessionStatus = "active" | "archived";

export type AIDraftingSession = {
  id: string;
  workspace_id: string;
  project_id: string;
  created_by: string;
  title: string;
  status: AIDraftingSessionStatus;
  active_context_snapshot_id?: string;
  last_message_at: string;
  created_at: string;
  updated_at: string;
};

export type AIDraftingSessionMessageRole = "user" | "assistant" | "system";

export type AIDraftingSessionMessage = {
  id: string;
  session_id: string;
  role: AIDraftingSessionMessageRole;
  content: string;
  created_at: string;
};

export type AIDraftingTimelineEventType =
  | "status"
  | "message"
  | "tool_call"
  | "tool_result"
  | "proposal"
  | "compact_boundary"
  | "context"
  | "error";

export type AIDraftingTimelineEvent = {
  id: string;
  session_id: string;
  event_type: AIDraftingTimelineEventType;
  title: string;
  detail?: string;
  status?: "queued" | "running" | "completed" | "failed" | "cancelled";
  created_at: string;
};

export type AIDraftingArtifactStatus =
  | "proposed"
  | "accepted"
  | "rejected"
  | "superseded";

export type AIDraftingArtifact = {
  id: string;
  session_id: string;
  title: string;
  kind: "source_patch" | "platform_variant" | "checklist" | "title_candidates";
  summary: string;
  target_platform?: PublishPlatform;
  status: AIDraftingArtifactStatus;
  created_at: string;
};

export type AIDraftingSessionDetail = {
  session: AIDraftingSession;
  messages: AIDraftingSessionMessage[];
  events: AIDraftingTimelineEvent[];
  artifacts: AIDraftingArtifact[];
};

export type AIDraftingSessionsResponse = {
  items: AIDraftingSession[];
};

export type RequirementStatus = ContractSchema<"RequirementStatus">;
export type WechatAccount = ContractSchema<"WechatAccount">;
export type SaveWechatAccountInput = ContractSchema<"SaveWechatAccountInput">;
export type WechatConnectionTestResult =
  ContractSchema<"WechatConnectionTestResult">;
export type XAccount = ContractSchema<"XAccount">;
export type SaveXAccountInput = ContractSchema<"SaveXAccountInput">;
export type XConnectionTestResult = ContractSchema<"XConnectionTestResult">;
export type DouyinAccount = ContractSchema<"DouyinAccount">;
export type ZhihuAccount = ContractSchema<"ZhihuAccount">;

export type BrowserSessionStatus = ContractSchema<"BrowserSessionStatus">;
export type BrowserSession = ContractSchema<"BrowserSession">;
export type StartBrowserSessionResult =
  ContractSchema<"StartBrowserSessionResult">;
export type StartPublishBrowserSessionResult =
  ContractSchema<"StartPublishBrowserSessionResult">;
export type CompleteBrowserSessionResult =
  ContractSchema<"CompleteBrowserSessionResult">;
export type CancelBrowserSessionResult =
  ContractSchema<"CancelBrowserSessionResult">;

export type ProjectListItem = ContractSchema<"ProjectListItem">;
export type ProjectDetail = ContractSchema<"ProjectDetail">;
export type ProjectPermissionSource = ContractSchema<"ProjectPermissionSource">;
export type ProjectCollaborator = ContractSchema<"ProjectCollaborator">;
export type ProjectCollaboratorsResponse =
  ContractSchema<"ProjectCollaboratorsResponse">;
export type ProjectActivityType = ContractSchema<"ProjectActivityType">;
export type ProjectActivity = ContractSchema<"ProjectActivity">;
export type ProjectActivitiesResponse =
  ContractSchema<"ProjectActivitiesResponse">;
export type ProjectComment = ContractSchema<"ProjectComment">;
export type ProjectCommentsResponse = ContractSchema<"ProjectCommentsResponse">;
export type CreateProjectCommentInput =
  ContractSchema<"CreateProjectCommentRequest">;
export type UpdateProjectCommentInput =
  ContractSchema<"UpdateProjectCommentRequest">;
export type ProjectVersion = ContractSchema<"ProjectVersion">;
export type ProjectVersionsResponse = ContractSchema<"ProjectVersionsResponse">;
export type RestoreProjectVersionResponse =
  ContractSchema<"RestoreProjectVersionResponse">;
export type ProjectShareLink = ContractSchema<"ProjectShareLink">;
export type ProjectShareLinkWithToken =
  ContractSchema<"ProjectShareLinkWithToken">;
export type ProjectShareLinksResponse =
  ContractSchema<"ProjectShareLinksResponse">;
export type CreateProjectShareLinkInput =
  ContractSchema<"CreateProjectShareLinkRequest">;
export type AcceptProjectShareLinkResponse =
  ContractSchema<"AcceptProjectShareLinkResponse">;
export type PaginatedProjects = ContractSchema<"PaginationProjects">;
export type ListDashboardProjectsOptions = {
  cursor?: string;
  limit?: number;
  status?: ProjectStatus;
  platform?: PublishPlatform;
};
export type ListWorkspaceProjectsOptions = {
  cursor?: string;
  limit?: number;
  status?: ProjectStatus;
  platform?: PublishPlatform;
};

export type WorkspaceRole = ContractSchema<"WorkspaceRole">;
export type WorkspaceStatus = ContractSchema<"WorkspaceStatus">;
export type CreateWorkspaceInput = ContractSchema<"CreateWorkspaceRequest">;
export type UpdateWorkspaceInput = ContractSchema<"UpdateWorkspaceRequest">;
export type AddWorkspaceMemberInput =
  ContractSchema<"AddWorkspaceMemberRequest">;
export type CreateWorkspaceInviteInput =
  ContractSchema<"CreateWorkspaceInviteRequest">;
export type AcceptWorkspaceInviteInput =
  ContractSchema<"AcceptWorkspaceInviteRequest">;
export type UpdateWorkspaceMemberInput =
  ContractSchema<"UpdateWorkspaceMemberRequest">;
export type Workspace = ContractSchema<"Workspace">;
export type WorkspacesResponse = ContractSchema<"WorkspacesResponse">;
export type WorkspaceMember = ContractSchema<"WorkspaceMember">;
export type WorkspaceMembersResponse =
  ContractSchema<"WorkspaceMembersResponse">;
export type WorkspaceInvite = ContractSchema<"WorkspaceInvite">;
export type WorkspaceInviteWithToken =
  ContractSchema<"WorkspaceInviteWithToken">;
export type WorkspaceInvitesResponse =
  ContractSchema<"WorkspaceInvitesResponse">;
export type WorkspaceActivity = ContractSchema<"WorkspaceActivity">;
export type WorkspaceActivityType = ContractSchema<"WorkspaceActivityType">;
export type WorkspaceActivitiesResponse =
  ContractSchema<"WorkspaceActivitiesResponse">;

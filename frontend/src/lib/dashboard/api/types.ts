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

export type CreateProjectInput = {
  title: string;
  source_content: string;
  summary?: string;
  cover_image_url?: string;
  platforms: PublishPlatform[];
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
export type ProjectCollaborator = ContractSchema<"ProjectCollaborator">;
export type ProjectCollaboratorsResponse =
  ContractSchema<"ProjectCollaboratorsResponse">;
export type ProjectActivityType =
  | "content_saved"
  | "comment_created"
  | "comment_resolved"
  | "collaborator_added"
  | "collaborator_role_changed"
  | "collaborator_removed"
  | "publish_requested"
  | "publish_queued"
  | "share_link_created"
  | "share_link_revoked"
  | "version_restored";
export type ProjectActivity = {
  id: string;
  project_id: string;
  actor_user_id: string;
  actor_username: string;
  actor_email: string;
  target_user_id?: string;
  target_username?: string;
  target_email?: string;
  event_type: ProjectActivityType;
  metadata: Record<string, unknown>;
  created_at: string;
};
export type ProjectActivitiesResponse = {
  items: ProjectActivity[];
};
export type ProjectComment = {
  id: string;
  project_id: string;
  author_id: string;
  author_username: string;
  author_email: string;
  body: string;
  anchor_text?: string;
  status: "open" | "resolved";
  metadata: Record<string, unknown>;
  created_at: string;
  resolved_at?: string;
};
export type ProjectCommentsResponse = {
  items: ProjectComment[];
};
export type CreateProjectCommentInput = {
  body: string;
  anchor_text?: string;
  metadata?: Record<string, unknown>;
};
export type UpdateProjectCommentInput = {
  status: "resolved";
};
export type ProjectVersion = {
  id: string;
  project_id: string;
  created_by: string;
  creator_username: string;
  creator_email: string;
  version_number: number;
  title: string;
  source: string;
  collab_document_id?: string;
  collab_seq: number;
  created_at: string;
};
export type ProjectVersionsResponse = {
  items: ProjectVersion[];
};
export type RestoreProjectVersionResponse = {
  project: ProjectDetail;
  version: ProjectVersion;
};
export type ProjectShareLink = {
  id: string;
  project_id: string;
  created_by: string;
  role: ProjectCollaboratorRole;
  status: "active" | "revoked";
  expires_at?: string;
  created_at: string;
  revoked_at?: string;
};
export type ProjectShareLinkWithToken = ProjectShareLink & {
  token: string;
  url: string;
};
export type ProjectShareLinksResponse = {
  items: ProjectShareLink[];
};
export type CreateProjectShareLinkInput = {
  role: ProjectCollaboratorRole;
  expires_at?: string;
};
export type PaginatedProjects = ContractSchema<"PaginationProjects">;
export type ListWorkspaceProjectsOptions = {
  page?: number;
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
export type UpdateWorkspaceMemberInput =
  ContractSchema<"UpdateWorkspaceMemberRequest">;
export type Workspace = ContractSchema<"Workspace">;
export type WorkspacesResponse = ContractSchema<"WorkspacesResponse">;
export type WorkspaceMember = ContractSchema<"WorkspaceMember">;
export type WorkspaceMembersResponse =
  ContractSchema<"WorkspaceMembersResponse">;
export type WorkspaceActivity = ContractSchema<"WorkspaceActivity">;
export type WorkspaceActivityType = ContractSchema<"WorkspaceActivityType">;
export type WorkspaceActivitiesResponse =
  ContractSchema<"WorkspaceActivitiesResponse">;

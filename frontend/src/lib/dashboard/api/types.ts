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
export type ProjectPermissionSource =
  ContractSchema<"ProjectPermissionSource">;
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

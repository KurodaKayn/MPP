export type DashboardStats = {
  total_users: number;
  total_projects: number;
  total_published_publications: number;
  total_failed_publications: number;
};

export type AdaptedContent = {
  schema_version?: number;
  format?: "html" | "markdown" | "text" | string;
  summary?: string;
  source_revision?: string;
  generated_by?: Record<string, unknown>;
  html?: string;
  markdown?: string;
  text?: string;
  assets?: Array<Record<string, unknown>>;
};

export type PublicationSummary = {
  id: string;
  platform: string;
  enabled: boolean;
  status: string;
  publish_url?: string;
};

export type PublicationDetail = PublicationSummary & {
  error_message?: string;
  config: Record<string, unknown>;
  adapted_content: AdaptedContent;
  remote_id?: string;
  retry_count: number;
  last_attempt_at?: string;
  published_at?: string;
  created_at: string;
  updated_at: string;
};

export type ProjectPublications = {
  project_id: string;
  items: PublicationDetail[];
};

export type PublishResult = {
  status: string;
  job_id?: string;
  platform?: string;
  queued_at?: string;
  remote_id?: string;
  publish_url?: string;
  error_message?: string;
  browser_session_id?: string;
};

export type PublishProjectOptions = {
  mode?: "manual";
};

export type CreateProjectInput = {
  title: string;
  source_content: string;
  summary?: string;
  cover_image_url?: string;
  platforms: string[];
};

export type UpdateProjectInput = CreateProjectInput;

export type SaveProjectContentInput = Omit<CreateProjectInput, "platforms">;

export type SaveProjectPlatformsInput = {
  platforms: string[];
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
  platforms: string[];
  actor?: {
    type: "system";
  };
};

export type UpdatePrepublishDraftInput = {
  adapted_content: AdaptedContent;
};

export type AIChatMessage = {
  role: "user" | "assistant";
  content: string;
};

export type AIEditContentStreamInput = {
  title?: string;
  content: string;
  message: string;
  conversation?: AIChatMessage[];
};

export type AIEditPrepublishStreamInput = {
  title?: string;
  platform: string;
  adapted_content: AdaptedContent;
  message: string;
  conversation?: AIChatMessage[];
};

export type AITextStreamOptions = {
  onChunk?: (chunk: string, accumulated: string) => void;
  signal?: AbortSignal;
};

export type RequirementStatus = {
  status: "passed" | "warning" | "failed" | "unknown";
  title: string;
  message: string;
};

export type WechatAccount = {
  platform: "wechat";
  app_id: string;
  has_app_secret: boolean;
  status: "unconfigured" | "untested" | "connected" | "failed";
  last_tested_at?: string;
  last_test_error?: string;
  updated_at?: string;
  ip_whitelist: RequirementStatus;
  account_auth: RequirementStatus;
};

export type SaveWechatAccountInput = {
  app_id: string;
  app_secret?: string;
};

export type WechatConnectionTestResult = {
  connected: boolean;
  status: "connected" | "failed";
  message: string;
  err_code?: number;
  err_msg?: string;
  tested_at: string;
  ip_whitelist: RequirementStatus;
  account_auth: RequirementStatus;
};

export type XAccount = {
  platform: "x";
  api_key?: string;
  expires_at?: string;
  username?: string;
  has_api_secret: boolean;
  has_access_token: boolean;
  has_access_token_secret: boolean;
  status: "unconfigured" | "untested" | "connected" | "failed";
  last_tested_at?: string;
  last_test_error?: string;
  updated_at?: string;
  account_auth: RequirementStatus;
  publish_access: RequirementStatus;
};

export type SaveXAccountInput = {
  api_key?: string;
  api_secret?: string;
  access_token?: string;
  access_token_secret?: string;
  username?: string;
};

export type XConnectionTestResult = {
  connected: boolean;
  status: "connected" | "failed";
  message: string;
  tested_at: string;
  user_id?: string;
  username?: string;
  name?: string;
  account_auth: RequirementStatus;
  publish_access: RequirementStatus;
};

export type DouyinAccount = {
  platform: "douyin";
  username?: string;
  avatar_url?: string;
  status: "unconfigured" | "untested" | "connected" | "failed";
  last_tested_at?: string;
  last_test_error?: string;
  updated_at?: string;
};

export type ZhihuAccount = {
  platform: "zhihu";
  username?: string;
  avatar_url?: string;
  status: "unconfigured" | "untested" | "connected" | "failed";
  last_tested_at?: string;
  last_test_error?: string;
  updated_at?: string;
};

export type BrowserSessionStatus =
  | "pending"
  | "ready"
  | "login_detected"
  | "capturing"
  | "connected"
  | "expired"
  | "failed";

export type BrowserSession = {
  session_id: string;
  platform: string;
  status: BrowserSessionStatus;
  stream_url?: string;
  stream_token_expires_at?: string;
  expires_at: string;
  message?: string;
};

export type StartBrowserSessionResult = {
  session_id: string;
  status: BrowserSessionStatus;
  stream_url: string;
  stream_token_expires_at: string;
  expires_at: string;
};

export type CompleteBrowserSessionResult = {
  session_id: string;
  platform: string;
  status: BrowserSessionStatus;
  account: {
    username: string;
    avatar_url: string;
  };
  message: string;
};

export type CancelBrowserSessionResult = {
  session_id: string;
  status: BrowserSessionStatus;
};

export type ProjectListItem = {
  id: string;
  user_id: string;
  title: string;
  status: string;
  created_at: string;
  updated_at: string;
  publications: PublicationSummary[];
};

export type ProjectDetail = ProjectListItem & {
  source_content: string;
};

export type PaginatedProjects = {
  items: ProjectListItem[];
  page: number;
  limit: number;
  total: number;
  total_pages: number;
};

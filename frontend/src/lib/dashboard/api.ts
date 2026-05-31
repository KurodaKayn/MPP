import { formatBearerToken, getStoredAuthToken } from "../auth/client";

export type DashboardStats = {
  total_users: number;
  total_projects: number;
  total_published_publications: number;
  total_failed_publications: number;
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

export type WaitForProjectPublicationsOptions = {
  intervalMs?: number;
  timeoutMs?: number;
  fetchProjectPublications?: typeof getProjectPublications;
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

type ApiErrorResponse = {
  message?: string;
  error?: {
    code?: string;
    message?: string;
  };
};

async function fetchDashboard<T>(
  path: string,
  init?: Omit<RequestInit, "headers" | "credentials">,
): Promise<T> {
  const headers = new Headers({
    Accept: "application/json",
  });
  const token = getStoredAuthToken();

  if (token) {
    headers.set("Authorization", formatBearerToken(token));
  }

  if (init?.body) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(path, {
    ...init,
    credentials: "same-origin",
    headers,
  });

  if (!response.ok) {
    let message = `请求失败 (${response.status})`;

    try {
      const body = (await response.json()) as ApiErrorResponse;
      message =
        body.error?.message || body.error?.code || body.message || message;
    } catch {
      // Keep the HTTP status fallback when the backend does not return JSON.
    }

    throw new Error(message);
  }

  return response.json() as Promise<T>;
}

async function streamDashboardText(
  path: string,
  body: unknown,
  options: AITextStreamOptions = {},
) {
  const headers = new Headers({
    Accept: "text/markdown, text/plain, application/json",
    "Content-Type": "application/json",
  });
  const token = getStoredAuthToken();

  if (token) {
    headers.set("Authorization", formatBearerToken(token));
  }

  const response = await fetch(path, {
    body: JSON.stringify(body),
    credentials: "same-origin",
    headers,
    method: "POST",
    signal: options.signal,
  });

  if (!response.ok) {
    let message = `请求失败 (${response.status})`;

    try {
      const errorBody = (await response.json()) as ApiErrorResponse;
      message =
        errorBody.error?.message ||
        errorBody.error?.code ||
        errorBody.message ||
        message;
    } catch {
      // Keep the HTTP status fallback when the backend does not return JSON.
    }

    throw new Error(message);
  }

  if (!response.body) {
    const text = await response.text();
    options.onChunk?.(text, text);
    return text;
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let accumulated = "";

  for (;;) {
    const { done, value } = await reader.read();
    if (done) {
      const trailing = decoder.decode();
      if (trailing) {
        accumulated += trailing;
        options.onChunk?.(trailing, accumulated);
      }
      if (!accumulated.trim()) {
        throw new Error("AI 没有返回内容，请换个说法再试。");
      }
      return accumulated;
    }

    const chunk = decoder.decode(value, { stream: true });
    if (!chunk) {
      continue;
    }
    accumulated += chunk;
    options.onChunk?.(chunk, accumulated);
  }
}

export function getDashboardStats() {
  return fetchDashboard<DashboardStats>("/api/user/dashboard/stats");
}

export function getDashboardProjects(limit = 8) {
  const params = new URLSearchParams({
    page: "1",
    limit: String(limit),
  });

  return fetchDashboard<PaginatedProjects>(
    `/api/user/dashboard/projects?${params.toString()}`,
  );
}

export function createDashboardProject(input: CreateProjectInput) {
  return fetchDashboard<ProjectListItem>("/api/user/dashboard/projects", {
    body: JSON.stringify(input),
    method: "POST",
  });
}

export function getDashboardProject(projectId: string) {
  return fetchDashboard<ProjectDetail>(
    `/api/user/dashboard/projects/${projectId}`,
  );
}

export function updateDashboardProject(
  projectId: string,
  input: UpdateProjectInput,
) {
  return fetchDashboard<ProjectDetail>(
    `/api/user/dashboard/projects/${projectId}`,
    {
      body: JSON.stringify(input),
      method: "PUT",
    },
  );
}

export function saveDashboardProjectContent(
  projectId: string,
  input: SaveProjectContentInput,
) {
  return fetchDashboard<ProjectDetail>(
    `/api/user/dashboard/projects/${projectId}/content`,
    {
      body: JSON.stringify(input),
      method: "PATCH",
    },
  );
}

export function saveDashboardProjectPlatforms(
  projectId: string,
  input: SaveProjectPlatformsInput,
) {
  return fetchDashboard<ProjectDetail>(
    `/api/user/dashboard/projects/${projectId}/platforms`,
    {
      body: JSON.stringify(input),
      method: "PATCH",
    },
  );
}

export function getProjectPublications(
  projectId: string,
  options?: GetProjectPublicationsOptions,
) {
  const query = options?.includeContent ? "?include_content=true" : "";

  return fetchDashboard<ProjectPublications>(
    `/api/user/dashboard/projects/${projectId}/publications${query}`,
  );
}

function defaultSleep(ms: number) {
  return new Promise<void>((resolve) => {
    globalThis.setTimeout(resolve, ms);
  });
}

function isPublishingStatus(status: string) {
  return status === "publishing";
}

export async function waitForProjectPublications(
  projectId: string,
  platforms: string[],
  options: WaitForProjectPublicationsOptions = {},
) {
  const timeoutMs = options.timeoutMs ?? 5 * 60 * 1000;
  const intervalMs = options.intervalMs ?? 2000;
  const fetchProjectPublications =
    options.fetchProjectPublications ?? getProjectPublications;
  const sleep = options.sleep ?? defaultSleep;
  const deadline = Date.now() + timeoutMs;
  const expectedPlatforms = new Set(platforms);

  let latestPublications = await fetchProjectPublications(projectId);

  while (Date.now() <= deadline) {
    const relevantPublications = latestPublications.items.filter((item) =>
      expectedPlatforms.has(item.platform),
    );
    if (
      relevantPublications.length === expectedPlatforms.size &&
      relevantPublications.every(
        (publication) => !isPublishingStatus(publication.status),
      )
    ) {
      return latestPublications;
    }

    await sleep(intervalMs);
    latestPublications = await fetchProjectPublications(projectId);
  }

  throw new Error("发布任务超时，请稍后刷新查看状态");
}

export function syncProjectPrepublish(
  projectId: string,
  input: SyncPrepublishInput,
) {
  return fetchDashboard<ProjectPublications>(
    `/api/user/dashboard/projects/${projectId}/prepublish/sync`,
    {
      body: JSON.stringify({
        actor: input.actor ?? { type: "system" },
        platforms: input.platforms,
      }),
      method: "POST",
    },
  );
}

export function updateProjectPrepublishDraft(
  projectId: string,
  platform: string,
  input: UpdatePrepublishDraftInput,
) {
  return fetchDashboard<ProjectPublications>(
    `/api/user/dashboard/projects/${projectId}/prepublish/${encodeURIComponent(platform)}`,
    {
      body: JSON.stringify(input),
      method: "PUT",
    },
  );
}

export function streamAIContentEdit(
  input: AIEditContentStreamInput,
  options?: AITextStreamOptions,
) {
  return streamDashboardText(
    "/api/user/dashboard/ai/content/edit/stream",
    input,
    options,
  );
}

export function streamAIPrepublishEdit(
  input: AIEditPrepublishStreamInput,
  options?: AITextStreamOptions,
) {
  return streamDashboardText(
    "/api/user/dashboard/ai/prepublish/edit/stream",
    input,
    options,
  );
}

export function publishProject(
  projectId: string,
  platform: string,
  options?: PublishProjectOptions,
) {
  const body = options?.mode ? { mode: options.mode, platform } : { platform };

  return fetchDashboard<PublishResult>(
    `/api/user/dashboard/projects/${projectId}/publish`,
    {
      body: JSON.stringify(body),
      method: "POST",
    },
  );
}

export function getWechatAccount() {
  return fetchDashboard<WechatAccount>(
    "/api/user/dashboard/settings/wechat/account",
  );
}

export function saveWechatAccount(input: SaveWechatAccountInput) {
  return fetchDashboard<WechatAccount>(
    "/api/user/dashboard/settings/wechat/account",
    {
      body: JSON.stringify(input),
      method: "PUT",
    },
  );
}

export function testWechatConnection(input: SaveWechatAccountInput) {
  return fetchDashboard<WechatConnectionTestResult>(
    "/api/user/dashboard/settings/wechat/test",
    {
      body: JSON.stringify(input),
      method: "POST",
    },
  );
}

export function getDouyinAccount() {
  return fetchDashboard<DouyinAccount>(
    "/api/user/dashboard/settings/douyin/account",
  );
}

export function getZhihuAccount() {
  return fetchDashboard<ZhihuAccount>(
    "/api/user/dashboard/settings/zhihu/account",
  );
}

export function startBrowserSession(platform: string) {
  return fetchDashboard<StartBrowserSessionResult>(
    `/api/user/dashboard/settings/platforms/${encodeURIComponent(platform)}/browser-session`,
    { method: "POST" },
  );
}

export function getBrowserSession(sessionId: string) {
  return fetchDashboard<BrowserSession>(
    `/api/user/dashboard/browser-sessions/${encodeURIComponent(sessionId)}`,
  );
}

export function completeBrowserSession(sessionId: string) {
  return fetchDashboard<CompleteBrowserSessionResult>(
    `/api/user/dashboard/browser-sessions/${encodeURIComponent(sessionId)}/complete`,
    { method: "POST" },
  );
}

export function cancelBrowserSession(sessionId: string) {
  return fetchDashboard<CancelBrowserSessionResult>(
    `/api/user/dashboard/browser-sessions/${encodeURIComponent(sessionId)}`,
    { method: "DELETE" },
  );
}

export function getXAccount() {
  return fetchDashboard<XAccount>("/api/user/dashboard/settings/x/account");
}

export function saveXAccount(input: SaveXAccountInput) {
  return fetchDashboard<XAccount>("/api/user/dashboard/settings/x/account", {
    body: JSON.stringify(input),
    method: "PUT",
  });
}

export function testXConnection(input: SaveXAccountInput) {
  return fetchDashboard<XConnectionTestResult>(
    "/api/user/dashboard/settings/x/test",
    {
      body: JSON.stringify(input),
      method: "POST",
    },
  );
}

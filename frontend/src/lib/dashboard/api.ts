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
  adapted_content: Record<string, unknown>;
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
  remote_id?: string;
  publish_url?: string;
  error_message?: string;
};

export type CreateProjectInput = {
  title: string;
  source_content: string;
  summary?: string;
  cover_image_url?: string;
  platforms: string[];
};

export type UpdateProjectInput = CreateProjectInput;

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

export function getProjectPublications(projectId: string) {
  return fetchDashboard<ProjectPublications>(
    `/api/user/dashboard/projects/${projectId}/publications`,
  );
}

export function publishProject(projectId: string, platform: string) {
  return fetchDashboard<PublishResult>(
    `/api/user/dashboard/projects/${projectId}/publish`,
    {
      body: JSON.stringify({ platform }),
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

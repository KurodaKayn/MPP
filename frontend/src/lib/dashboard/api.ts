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

export type ProjectListItem = {
  id: string;
  user_id: string;
  title: string;
  status: string;
  created_at: string;
  updated_at: string;
  publications: PublicationSummary[];
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

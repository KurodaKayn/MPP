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
  error?: {
    code?: string;
    message?: string;
  };
};

async function fetchDashboard<T>(path: string): Promise<T> {
  const response = await fetch(path, {
    headers: {
      Accept: "application/json",
    },
  });

  if (!response.ok) {
    let message = `请求失败 (${response.status})`;

    try {
      const body = (await response.json()) as ApiErrorResponse;
      message = body.error?.message || body.error?.code || message;
    } catch {
      // Keep the HTTP status fallback when the backend does not return JSON.
    }

    throw new Error(message);
  }

  return response.json() as Promise<T>;
}

export function getDashboardStats() {
  return fetchDashboard<DashboardStats>("/api/admin/dashboard/stats");
}

export function getDashboardProjects(limit = 8) {
  const params = new URLSearchParams({
    page: "1",
    limit: String(limit),
  });

  return fetchDashboard<PaginatedProjects>(
    `/api/admin/dashboard/projects?${params.toString()}`,
  );
}

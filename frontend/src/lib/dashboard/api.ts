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
  message?: string;
  error?: {
    code?: string;
    message?: string;
  };
};

const authTokenStorageKeys = [
  "sevenoxcloud.auth_token",
  "auth_token",
  "access_token",
];

function formatBearerToken(token: string) {
  return token.toLowerCase().startsWith("bearer ") ? token : `Bearer ${token}`;
}

function getStorageToken(storage: Storage) {
  for (const key of authTokenStorageKeys) {
    const token = storage.getItem(key);
    if (token) {
      return token;
    }
  }

  return null;
}

function getStoredAuthToken() {
  if (typeof window === "undefined") {
    return null;
  }

  for (const getStorage of [
    () => window.localStorage,
    () => window.sessionStorage,
  ]) {
    try {
      const token = getStorageToken(getStorage());
      if (token) {
        return token;
      }
    } catch {
      // Some privacy modes can deny Web Storage access.
    }
  }

  return null;
}

async function fetchDashboard<T>(path: string): Promise<T> {
  const headers = new Headers({
    Accept: "application/json",
  });
  const token = getStoredAuthToken();

  if (token) {
    headers.set("Authorization", formatBearerToken(token));
  }

  const response = await fetch(path, {
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

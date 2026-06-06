import { clearAuthSession, clearServerAuthSession } from "@/lib/auth/client";
import type { AITextStreamOptions } from "./types";

type ApiErrorResponse = {
  message?: string;
  error?: {
    code?: string;
    message?: string;
  };
};

const selectedWorkspaceStorageKey = "mpp.dashboard.selectedWorkspaceId";

function getStoredWorkspaceId() {
  if (typeof window === "undefined") {
    return "";
  }

  try {
    return (
      window.localStorage.getItem(selectedWorkspaceStorageKey)?.trim() ?? ""
    );
  } catch {
    return "";
  }
}

function pathWithWorkspaceContext(path: string, workspaceId: string) {
  if (!workspaceId || !path.startsWith("/api/user/dashboard")) {
    return path;
  }

  const [pathname, query = ""] = path.split("?", 2);
  const params = new URLSearchParams(query);
  if (!params.has("workspace_id")) {
    params.set("workspace_id", workspaceId);
  }
  const nextQuery = params.toString();
  return nextQuery ? `${pathname}?${nextQuery}` : pathname;
}

async function getDashboardErrorMessage(response: Response) {
  const fallback = `Request failed (${response.status})`;

  try {
    const body = (await response.json()) as ApiErrorResponse;
    return body.error?.message || body.error?.code || body.message || fallback;
  } catch {
    return fallback;
  }
}

function isExpiredAuthError(response: Response, message: string) {
  return (
    response.status === 401 ||
    message.trim().toLowerCase() === "invalid or expired jwt"
  );
}

async function createDashboardError(response: Response) {
  const message = await getDashboardErrorMessage(response);

  if (isExpiredAuthError(response, message) && typeof window !== "undefined") {
    clearAuthSession();
    await clearServerAuthSession();
  }

  return new Error(message);
}

export async function fetchDashboard<T>(
  path: string,
  init?: Omit<RequestInit, "headers" | "credentials">,
): Promise<T> {
  const headers = new Headers({
    Accept: "application/json",
  });

  if (init?.body) {
    headers.set("Content-Type", "application/json");
  }
  const workspaceId = getStoredWorkspaceId();
  if (workspaceId) {
    headers.set("X-Workspace-ID", workspaceId);
  }

  const response = await fetch(pathWithWorkspaceContext(path, workspaceId), {
    ...init,
    credentials: "same-origin",
    headers,
  });

  if (!response.ok) {
    throw await createDashboardError(response);
  }

  return response.json() as Promise<T>;
}

export async function fetchDashboardNoContent(
  path: string,
  init?: Omit<RequestInit, "headers" | "credentials">,
): Promise<void> {
  const headers = new Headers({
    Accept: "application/json",
  });

  if (init?.body) {
    headers.set("Content-Type", "application/json");
  }
  const workspaceId = getStoredWorkspaceId();
  if (workspaceId) {
    headers.set("X-Workspace-ID", workspaceId);
  }

  const response = await fetch(pathWithWorkspaceContext(path, workspaceId), {
    ...init,
    credentials: "same-origin",
    headers,
  });

  if (!response.ok) {
    throw await createDashboardError(response);
  }
}

export async function streamDashboardText(
  path: string,
  body: unknown,
  options: AITextStreamOptions = {},
) {
  const headers = new Headers({
    Accept: "text/markdown, text/plain, application/json",
    "Content-Type": "application/json",
  });
  const workspaceId = getStoredWorkspaceId();
  if (workspaceId) {
    headers.set("X-Workspace-ID", workspaceId);
  }

  const response = await fetch(pathWithWorkspaceContext(path, workspaceId), {
    body: JSON.stringify(body),
    credentials: "same-origin",
    headers,
    method: "POST",
    signal: options.signal,
  });

  if (!response.ok) {
    throw await createDashboardError(response);
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
        throw new Error(
          "AI returned no content. Please try a different prompt.",
        );
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

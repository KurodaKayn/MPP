import { clearAuthSession, clearServerAuthSession } from "@/lib/auth/client";
import type {
  AIDraftingStreamEvent,
  AIDraftingStreamOptions,
  AITextStreamOptions,
} from "./types";

type ApiErrorResponse = {
  message?: string;
  error?: {
    code?: string;
    message?: string;
  };
};

type DashboardRequestInit = Omit<RequestInit, "headers" | "credentials"> & {
  workspaceId?: string | null;
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

function resolveWorkspaceContext(workspaceId: string | null | undefined) {
  if (workspaceId === null) {
    return "";
  }
  if (typeof workspaceId === "string") {
    return workspaceId.trim();
  }
  return getStoredWorkspaceId();
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
  init?: DashboardRequestInit,
): Promise<T> {
  const { workspaceId: workspaceIdOption, ...fetchInit } = init ?? {};
  const headers = new Headers({
    Accept: "application/json",
  });

  if (fetchInit.body) {
    headers.set("Content-Type", "application/json");
  }
  const workspaceId = resolveWorkspaceContext(workspaceIdOption);
  if (workspaceId) {
    headers.set("X-Workspace-ID", workspaceId);
  }

  const response = await fetch(pathWithWorkspaceContext(path, workspaceId), {
    ...fetchInit,
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
  init?: DashboardRequestInit,
): Promise<void> {
  const { workspaceId: workspaceIdOption, ...fetchInit } = init ?? {};
  const headers = new Headers({
    Accept: "application/json",
  });

  if (fetchInit.body) {
    headers.set("Content-Type", "application/json");
  }
  const workspaceId = resolveWorkspaceContext(workspaceIdOption);
  if (workspaceId) {
    headers.set("X-Workspace-ID", workspaceId);
  }

  const response = await fetch(pathWithWorkspaceContext(path, workspaceId), {
    ...fetchInit,
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

export async function streamDashboardEvents(
  path: string,
  body: unknown,
  options: AIDraftingStreamOptions = {},
) {
  const headers = new Headers({
    Accept: "text/event-stream, application/json",
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
    return [];
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  const events: AIDraftingStreamEvent[] = [];

  const drain = (final = false) => {
    for (;;) {
      const separator = buffer.indexOf("\n\n");
      if (separator < 0) {
        if (final && buffer.trim()) {
          consumeSSEBlock(buffer);
          buffer = "";
        }
        return;
      }
      const block = buffer.slice(0, separator);
      buffer = buffer.slice(separator + 2);
      consumeSSEBlock(block);
    }
  };

  const consumeSSEBlock = (block: string) => {
    const lines = block.split(/\r?\n/);
    let eventType = "message";
    const dataLines: string[] = [];
    for (const line of lines) {
      if (line.startsWith("event:")) {
        eventType = line.slice("event:".length).trim();
      } else if (line.startsWith("data:")) {
        dataLines.push(line.slice("data:".length).trimStart());
      }
    }
    if (dataLines.length === 0) {
      return;
    }
    const payload = JSON.parse(dataLines.join("\n")) as Record<string, unknown>;
    const event: AIDraftingStreamEvent = {
      event_type: eventType,
      payload,
    };
    events.push(event);
    options.onEvent?.(event);
  };

  for (;;) {
    const { done, value } = await reader.read();
    if (done) {
      const trailing = decoder.decode();
      if (trailing) {
        buffer += trailing;
      }
      drain(true);
      return events;
    }
    const chunk = decoder.decode(value, { stream: true });
    if (!chunk) {
      continue;
    }
    buffer += chunk.replace(/\r\n/g, "\n");
    drain();
  }
}

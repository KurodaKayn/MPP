import {
  clearAuthSession,
  clearServerAuthSession,
  subscribeAuthChanged,
} from "@/lib/auth/client";
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
  cacheTtlMs?: number;
  workspaceId?: string | null;
};

export type DashboardGetCacheInvalidation = {
  path?: string;
  pathPrefix?: string;
  workspaceId?: string | null;
};

type DashboardGetCacheEntry<T = unknown> = {
  expiresAt: number;
  path: string;
  promise: Promise<T>;
  workspaceId: string;
};

const defaultDashboardGetCacheTtlMs = 10_000;
const selectedWorkspaceStorageKey = "mpp.dashboard.selectedWorkspaceId";
const authUserStorageKey = "sevenoxcloud.auth_user";
const dashboardGetCache = new Map<string, DashboardGetCacheEntry>();
let dashboardGetCacheTtlMs = defaultDashboardGetCacheTtlMs;
let dashboardGetCacheAuthScope: string | null = null;
let unsubscribeDashboardGetCacheAuthChanges: (() => void) | null = null;

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

function getStoredAuthUser(storage: Storage) {
  try {
    return storage.getItem(authUserStorageKey)?.trim() ?? "";
  } catch {
    return "";
  }
}

function getDashboardGetCacheAuthScope() {
  if (typeof window === "undefined") {
    return "";
  }

  try {
    const localAuthUser = getStoredAuthUser(window.localStorage);
    if (localAuthUser) {
      return localAuthUser;
    }
  } catch {
    // Some privacy modes can deny Web Storage access.
  }

  try {
    return getStoredAuthUser(window.sessionStorage);
  } catch {
    return "";
  }
}

function syncDashboardGetCacheAuthScope() {
  const nextAuthScope = getDashboardGetCacheAuthScope();
  if (dashboardGetCacheAuthScope === null) {
    dashboardGetCacheAuthScope = nextAuthScope;
    return;
  }
  if (dashboardGetCacheAuthScope !== nextAuthScope) {
    dashboardGetCache.clear();
    dashboardGetCacheAuthScope = nextAuthScope;
  }
}

function normalizeCacheTtlMs(ttlMs: number) {
  return Number.isFinite(ttlMs) ? Math.max(0, ttlMs) : 0;
}

function normalizeRequestMethod(method: string | undefined) {
  return (method ?? "GET").trim().toUpperCase();
}

function dashboardGetCacheKey(path: string, workspaceId: string) {
  return JSON.stringify({ path, workspaceId });
}

function isDashboardGetCacheEnabled() {
  return typeof window !== "undefined";
}

function ensureDashboardGetCacheAuthSubscription() {
  if (
    !isDashboardGetCacheEnabled() ||
    unsubscribeDashboardGetCacheAuthChanges
  ) {
    return;
  }

  unsubscribeDashboardGetCacheAuthChanges = subscribeAuthChanged(() => {
    clearDashboardGetCache();
  });
}

export function setDashboardGetCacheTtlMs(ttlMs: number) {
  dashboardGetCacheTtlMs = normalizeCacheTtlMs(ttlMs);
  clearDashboardGetCache();
}

export function clearDashboardGetCache() {
  dashboardGetCache.clear();
  dashboardGetCacheAuthScope = null;
}

export function invalidateDashboardGetCache(
  invalidation: DashboardGetCacheInvalidation = {},
) {
  const workspaceId =
    invalidation.workspaceId === undefined
      ? undefined
      : resolveWorkspaceContext(invalidation.workspaceId);

  for (const [key, entry] of dashboardGetCache) {
    if (workspaceId !== undefined && entry.workspaceId !== workspaceId) {
      continue;
    }
    if (invalidation.path && entry.path !== invalidation.path) {
      continue;
    }
    if (
      invalidation.pathPrefix &&
      !entry.path.startsWith(invalidation.pathPrefix)
    ) {
      continue;
    }
    dashboardGetCache.delete(key);
  }
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
    clearDashboardGetCache();
    clearAuthSession();
    await clearServerAuthSession();
  }

  return new Error(message);
}

async function requestDashboardJson<T>(
  path: string,
  init: RequestInit,
): Promise<T> {
  const response = await fetch(path, init);

  if (!response.ok) {
    throw await createDashboardError(response);
  }

  const method = normalizeRequestMethod(init.method);
  if (method !== "GET") {
    invalidateDashboardGetCache();
  }

  return response.json() as Promise<T>;
}

export async function fetchDashboard<T>(
  path: string,
  init?: DashboardRequestInit,
): Promise<T> {
  const {
    cacheTtlMs: cacheTtlMsOption,
    workspaceId: workspaceIdOption,
    ...fetchInit
  } = init ?? {};
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
  const requestPath = pathWithWorkspaceContext(path, workspaceId);
  const requestInit = {
    ...fetchInit,
    credentials: "same-origin" as const,
    headers,
  };
  const method = normalizeRequestMethod(fetchInit.method);

  if (method !== "GET") {
    return requestDashboardJson<T>(requestPath, requestInit);
  }

  if (!isDashboardGetCacheEnabled()) {
    return requestDashboardJson<T>(requestPath, requestInit);
  }

  ensureDashboardGetCacheAuthSubscription();
  syncDashboardGetCacheAuthScope();
  const ttlMs =
    cacheTtlMsOption === undefined
      ? dashboardGetCacheTtlMs
      : normalizeCacheTtlMs(cacheTtlMsOption);
  if (ttlMs <= 0) {
    return requestDashboardJson<T>(requestPath, requestInit);
  }

  const now = Date.now();
  const cacheKey = dashboardGetCacheKey(requestPath, workspaceId);
  const cached = dashboardGetCache.get(cacheKey);
  if (cached && cached.expiresAt > now) {
    return cached.promise as Promise<T>;
  }
  if (cached) {
    dashboardGetCache.delete(cacheKey);
  }

  const promise = requestDashboardJson<T>(requestPath, requestInit);
  dashboardGetCache.set(cacheKey, {
    expiresAt: now + ttlMs,
    path: requestPath,
    promise,
    workspaceId,
  });

  try {
    const data = await promise;
    const entry = dashboardGetCache.get(cacheKey);
    if (entry?.promise === promise) {
      entry.expiresAt = Date.now() + ttlMs;
    }
    return data;
  } catch (error) {
    const entry = dashboardGetCache.get(cacheKey);
    if (entry?.promise === promise) {
      dashboardGetCache.delete(cacheKey);
    }
    throw error;
  }
}

export async function fetchDashboardNoContent(
  path: string,
  init?: DashboardRequestInit,
): Promise<void> {
  const {
    cacheTtlMs: _cacheTtlMsOption,
    workspaceId: workspaceIdOption,
    ...fetchInit
  } = init ?? {};
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
  const requestPath = pathWithWorkspaceContext(path, workspaceId);

  const response = await fetch(requestPath, {
    ...fetchInit,
    credentials: "same-origin",
    headers,
  });

  if (!response.ok) {
    throw await createDashboardError(response);
  }

  const method = normalizeRequestMethod(fetchInit.method);
  if (method !== "GET") {
    invalidateDashboardGetCache();
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

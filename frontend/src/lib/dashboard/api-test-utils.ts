import { afterEach, beforeEach, vi } from "vitest";
import { clearDashboardGetCache, setDashboardGetCacheTtlMs } from "./api";

export function jsonResponse(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    ...init,
  });
}

export function textStreamResponse(chunks: string[], init?: ResponseInit) {
  return new Response(
    new ReadableStream({
      start(controller) {
        for (const chunk of chunks) {
          controller.enqueue(new TextEncoder().encode(chunk));
        }
        controller.close();
      },
    }),
    {
      headers: { "content-type": "text/markdown" },
      ...init,
    },
  );
}

export function setupDashboardApiTest() {
  beforeEach(() => {
    window.localStorage.clear();
    window.sessionStorage.clear();
    setDashboardGetCacheTtlMs(10_000);
    clearDashboardGetCache();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    setDashboardGetCacheTtlMs(10_000);
    clearDashboardGetCache();
  });
}

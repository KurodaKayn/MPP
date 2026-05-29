// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getDashboardProjects, getDashboardStats } from "./api";

function jsonResponse(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    ...init,
  });
}

describe("dashboard api client", () => {
  beforeEach(() => {
    window.localStorage.clear();
    window.sessionStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("sends same-origin requests with a bearer token from local storage", async () => {
    const stats = {
      total_failed_publications: 0,
      total_projects: 2,
      total_published_publications: 1,
      total_users: 1,
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(stats));
    vi.stubGlobal("fetch", fetchMock);
    window.localStorage.setItem("sevenoxcloud.auth_token", "local-token");

    await expect(getDashboardStats()).resolves.toEqual(stats);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/stats",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
    const [, init] = fetchMock.mock.calls[0];
    expect(init).toBeDefined();
    const headers = init!.headers as Headers;
    expect(headers.get("Authorization")).toBe("Bearer local-token");
  });

  it("falls back to session storage when local storage is unavailable", async () => {
    const localStorageDescriptor = Object.getOwnPropertyDescriptor(
      window,
      "localStorage",
    );
    Object.defineProperty(window, "localStorage", {
      configurable: true,
      get: () => {
        throw new Error("blocked");
      },
    });

    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse({ items: [] }),
    );
    vi.stubGlobal("fetch", fetchMock);
    window.sessionStorage.setItem("access_token", "Bearer session-token");

    try {
      await getDashboardProjects(12);
    } finally {
      if (localStorageDescriptor) {
        Object.defineProperty(window, "localStorage", localStorageDescriptor);
      }
    }

    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/user/dashboard/projects?page=1&limit=12");
    expect(init).toBeDefined();
    const headers = init!.headers as Headers;
    expect(headers.get("Authorization")).toBe("Bearer session-token");
  });

  it("uses backend error messages from JSON responses", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse(
        { error: { code: "forbidden", message: "not your project" } },
        { status: 403 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(getDashboardStats()).rejects.toThrow("not your project");
  });

  it("falls back to the HTTP status when an error response is not JSON", async () => {
    const fetchMock = vi.fn<typeof fetch>(
      async () => new Response("service unavailable", { status: 503 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(getDashboardStats()).rejects.toThrow("请求失败 (503)");
  });
});

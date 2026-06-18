// @vitest-environment jsdom

import { describe, expect, it, vi } from "vitest";
import { setAuthSession } from "@/lib/auth/client";
import {
  createDashboardProject,
  deleteDashboardProject,
  getDashboardProjects,
  getDashboardStats,
  setDashboardGetCacheTtlMs,
} from "./api";
import { jsonResponse, setupDashboardApiTest } from "./api-test-utils";

describe("dashboard api client requests", () => {
  setupDashboardApiTest();

  it("sends same-origin requests without exposing legacy stored tokens", async () => {
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
    expect(headers.get("Authorization")).toBeNull();
  });

  it("does not consult session tokens when local storage is unavailable", async () => {
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
    expect(path).toBe("/api/user/dashboard/projects?limit=12");
    expect(init).toBeDefined();
    const headers = init!.headers as Headers;
    expect(headers.get("Authorization")).toBeNull();
  });

  it("adds the selected workspace context to dashboard requests", async () => {
    const stats = {
      total_failed_publications: 0,
      total_projects: 1,
      total_published_publications: 0,
      total_users: 1,
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(stats));
    vi.stubGlobal("fetch", fetchMock);
    window.localStorage.setItem(
      "mpp.dashboard.selectedWorkspaceId",
      "workspace-1",
    );

    await expect(getDashboardStats()).resolves.toEqual(stats);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/stats?workspace_id=workspace-1",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
    const [, init] = fetchMock.mock.calls[0];
    const headers = init!.headers as Headers;
    expect(headers.get("X-Workspace-ID")).toBe("workspace-1");
  });

  it("omits the selected workspace context when explicitly disabled", async () => {
    const fetchMock = vi.fn<typeof fetch>(
      async () => new Response(null, { status: 204 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    window.localStorage.setItem(
      "mpp.dashboard.selectedWorkspaceId",
      "workspace-1",
    );

    await expect(
      deleteDashboardProject("project-1", { workspaceId: null }),
    ).resolves.toBeUndefined();

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "DELETE",
      }),
    );
    const [, init] = fetchMock.mock.calls[0];
    const headers = init!.headers as Headers;
    expect(headers.get("X-Workspace-ID")).toBeNull();
  });

  it("uses an explicit workspace context when provided", async () => {
    const fetchMock = vi.fn<typeof fetch>(
      async () => new Response(null, { status: 204 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    window.localStorage.setItem(
      "mpp.dashboard.selectedWorkspaceId",
      "workspace-1",
    );

    await expect(
      deleteDashboardProject("project-1", { workspaceId: "workspace-2" }),
    ).resolves.toBeUndefined();

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1?workspace_id=workspace-2",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "DELETE",
      }),
    );
    const [, init] = fetchMock.mock.calls[0];
    const headers = init!.headers as Headers;
    expect(headers.get("X-Workspace-ID")).toBe("workspace-2");
  });

  it("reuses dashboard GET responses within the configured cache TTL", async () => {
    const stats = {
      total_failed_publications: 0,
      total_projects: 1,
      total_published_publications: 0,
      total_users: 1,
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(stats));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getDashboardStats()).resolves.toEqual(stats);
    await expect(getDashboardStats()).resolves.toEqual(stats);

    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("refreshes dashboard GET responses after the cache TTL expires", async () => {
    vi.useFakeTimers();
    setDashboardGetCacheTtlMs(100);
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        jsonResponse({
          total_failed_publications: 0,
          total_projects: 1,
          total_published_publications: 0,
          total_users: 1,
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          total_failed_publications: 0,
          total_projects: 2,
          total_published_publications: 1,
          total_users: 1,
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await expect(getDashboardStats()).resolves.toMatchObject({
      total_projects: 1,
    });
    vi.advanceTimersByTime(101);

    await expect(getDashboardStats()).resolves.toMatchObject({
      total_projects: 2,
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("separates dashboard GET cache entries by workspace context", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        jsonResponse({
          total_failed_publications: 0,
          total_projects: 1,
          total_published_publications: 0,
          total_users: 1,
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          total_failed_publications: 0,
          total_projects: 2,
          total_published_publications: 0,
          total_users: 1,
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    window.localStorage.setItem(
      "mpp.dashboard.selectedWorkspaceId",
      "workspace-1",
    );
    await expect(getDashboardStats()).resolves.toMatchObject({
      total_projects: 1,
    });
    window.localStorage.setItem(
      "mpp.dashboard.selectedWorkspaceId",
      "workspace-2",
    );
    await expect(getDashboardStats()).resolves.toMatchObject({
      total_projects: 2,
    });
    window.localStorage.setItem(
      "mpp.dashboard.selectedWorkspaceId",
      "workspace-1",
    );
    await expect(getDashboardStats()).resolves.toMatchObject({
      total_projects: 1,
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/user/dashboard/stats?workspace_id=workspace-1",
    );
    expect(fetchMock.mock.calls[1][0]).toBe(
      "/api/user/dashboard/stats?workspace_id=workspace-2",
    );
  });

  it("clears cached dashboard GET responses when auth changes with the same display user", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        jsonResponse({
          total_failed_publications: 0,
          total_projects: 1,
          total_published_publications: 0,
          total_users: 1,
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          total_failed_publications: 0,
          total_projects: 2,
          total_published_publications: 0,
          total_users: 1,
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    setAuthSession({ username: "Creator" });
    await expect(getDashboardStats()).resolves.toMatchObject({
      total_projects: 1,
    });

    setAuthSession({ username: "Creator" });
    await expect(getDashboardStats()).resolves.toMatchObject({
      total_projects: 2,
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("does not reuse cached responses for non-GET requests", async () => {
    const firstProject = {
      cover_image_url: null,
      created_at: "2026-06-01T00:00:00Z",
      id: "project-1",
      owner_id: "user-1",
      platforms: [],
      status: "draft",
      title: "First Project",
      updated_at: "2026-06-01T00:00:00Z",
    };
    const secondProject = {
      ...firstProject,
      id: "project-2",
      title: "Second Project",
    };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(firstProject))
      .mockResolvedValueOnce(jsonResponse(secondProject));
    vi.stubGlobal("fetch", fetchMock);

    const input = {
      platforms: ["wechat" as const],
      source_content: "Body",
      title: "Project",
    };
    await expect(createDashboardProject(input)).resolves.toMatchObject({
      id: "project-1",
    });
    await expect(createDashboardProject(input)).resolves.toMatchObject({
      id: "project-2",
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[0][1]).toEqual(
      expect.objectContaining({ method: "POST" }),
    );
    expect(fetchMock.mock.calls[1][1]).toEqual(
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("invalidates cached dashboard GET responses after mutations", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse({ items: [{ id: "project-1" }] }))
      .mockResolvedValueOnce(
        jsonResponse({
          cover_image_url: null,
          created_at: "2026-06-01T00:00:00Z",
          id: "project-2",
          owner_id: "user-1",
          platforms: [],
          status: "draft",
          title: "New Project",
          updated_at: "2026-06-01T00:00:00Z",
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          items: [{ id: "project-1" }, { id: "project-2" }],
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await expect(getDashboardProjects(8)).resolves.toEqual({
      items: [{ id: "project-1" }],
    });
    await expect(getDashboardProjects(8)).resolves.toEqual({
      items: [{ id: "project-1" }],
    });
    await createDashboardProject({
      platforms: ["wechat"],
      source_content: "Body",
      title: "New Project",
    });
    await expect(getDashboardProjects(8)).resolves.toEqual({
      items: [{ id: "project-1" }, { id: "project-2" }],
    });

    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/user/dashboard/projects?limit=8",
    );
    expect(fetchMock.mock.calls[1][0]).toBe("/api/user/dashboard/projects");
    expect(fetchMock.mock.calls[2][0]).toBe(
      "/api/user/dashboard/projects?limit=8",
    );
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

  it("clears auth sessions when the dashboard token expires", async () => {
    const authChanges: string[] = [];
    const onAuthChange = () => authChanges.push("changed");
    window.addEventListener("sevenoxcloud.auth_changed", onAuthChange);
    window.localStorage.setItem("sevenoxcloud.auth_token", "expired-token");
    document.cookie = "sevenoxcloud.auth_token=expired-token; path=/";

    const fetchMock = vi.fn<typeof fetch>(async (input) => {
      if (input === "/api/auth/session") {
        return jsonResponse({ ok: true });
      }

      return jsonResponse(
        { message: "invalid or expired jwt" },
        { status: 401 },
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    try {
      await expect(getDashboardStats()).rejects.toThrow(
        "invalid or expired jwt",
      );
    } finally {
      window.removeEventListener("sevenoxcloud.auth_changed", onAuthChange);
    }

    expect(window.localStorage.getItem("sevenoxcloud.auth_token")).toBeNull();
    expect(document.cookie).not.toContain("sevenoxcloud.auth_token=");
    expect(authChanges).toEqual(["changed"]);
    expect(fetchMock).toHaveBeenLastCalledWith(
      "/api/auth/session",
      expect.objectContaining({
        credentials: "same-origin",
        method: "DELETE",
      }),
    );
  });

  it("clears cached dashboard GET data when auth expires", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        jsonResponse({
          total_failed_publications: 0,
          total_projects: 1,
          total_published_publications: 0,
          total_users: 1,
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({ message: "invalid or expired jwt" }, { status: 401 }),
      )
      .mockResolvedValueOnce(jsonResponse({ ok: true }))
      .mockResolvedValueOnce(
        jsonResponse({
          total_failed_publications: 0,
          total_projects: 2,
          total_published_publications: 0,
          total_users: 1,
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await expect(getDashboardStats()).resolves.toMatchObject({
      total_projects: 1,
    });
    await expect(getDashboardProjects(8)).rejects.toThrow(
      "invalid or expired jwt",
    );
    await expect(getDashboardStats()).resolves.toMatchObject({
      total_projects: 2,
    });

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/user/dashboard/stats",
      expect.objectContaining({ credentials: "same-origin" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/user/dashboard/projects?limit=8",
      expect.objectContaining({ credentials: "same-origin" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "/api/auth/session",
      expect.objectContaining({
        credentials: "same-origin",
        method: "DELETE",
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      4,
      "/api/user/dashboard/stats",
      expect.objectContaining({ credentials: "same-origin" }),
    );
  });

  it("falls back to the HTTP status when an error response is not JSON", async () => {
    const fetchMock = vi.fn<typeof fetch>(
      async () => new Response("service unavailable", { status: 503 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(getDashboardStats()).rejects.toThrow("Request failed (503)");
  });
});

// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  cancelBrowserSession,
  completeBrowserSession,
  createDashboardProject,
  getBrowserSession,
  getDashboardProject,
  getDashboardProjects,
  getDashboardStats,
  getDouyinAccount,
  getProjectPublications,
  getXAccount,
  getWechatAccount,
  publishProject,
  saveXAccount,
  saveWechatAccount,
  startBrowserSession,
  streamAIContentEdit,
  streamAIPrepublishEdit,
  syncProjectPrepublish,
  waitForProjectPublications,
  testWechatConnection,
  testXConnection,
  updateDashboardProject,
} from "./api";
import type { ProjectPublications } from "./api";

function jsonResponse(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    ...init,
  });
}

function textStreamResponse(chunks: string[], init?: ResponseInit) {
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

  it("fetches publication details for a project", async () => {
    const publications = {
      items: [
        {
          adapted_content: { summary: "ready" },
          config: {},
          created_at: "2026-05-29T12:00:00Z",
          enabled: true,
          id: "pub-1",
          platform: "wechat",
          retry_count: 0,
          status: "adapted",
          updated_at: "2026-05-29T12:00:00Z",
        },
      ],
      project_id: "project-1",
    };
    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse(publications),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      getProjectPublications("project-1", { includeContent: true }),
    ).resolves.toEqual(publications);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1/publications?include_content=true",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
  });

  it("syncs platform prepublish drafts for a project", async () => {
    const publications = {
      items: [
        {
          adapted_content: {
            format: "markdown",
            markdown: "## Body",
            schema_version: 1,
          },
          config: {},
          created_at: "2026-05-29T12:00:00Z",
          enabled: true,
          id: "pub-1",
          platform: "zhihu",
          retry_count: 0,
          status: "adapted",
          updated_at: "2026-05-29T12:00:00Z",
        },
      ],
      project_id: "project-1",
    };
    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse(publications),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      syncProjectPrepublish("project-1", {
        platforms: ["zhihu"],
      }),
    ).resolves.toEqual(publications);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1/prepublish/sync",
      expect.objectContaining({
        body: JSON.stringify({
          actor: { type: "system" },
          platforms: ["zhihu"],
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "POST",
      }),
    );
  });

  it("streams AI content edit chunks", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () =>
      textStreamResponse(["hello ", "**world**"]),
    );
    vi.stubGlobal("fetch", fetchMock);
    const chunks: string[] = [];

    await expect(
      streamAIContentEdit(
        {
          content: "hello world",
          message: "bold world",
          title: "Draft",
        },
        {
          onChunk: (chunk) => chunks.push(chunk),
        },
      ),
    ).resolves.toBe("hello **world**");

    expect(chunks).toEqual(["hello ", "**world**"]);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/ai/content/edit/stream",
      expect.objectContaining({
        body: JSON.stringify({
          content: "hello world",
          message: "bold world",
          title: "Draft",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "POST",
      }),
    );
  });

  it("streams AI prepublish edit chunks", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () =>
      textStreamResponse(["## ", "Draft"]),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      streamAIPrepublishEdit({
        adapted_content: {
          format: "markdown",
          markdown: "# Draft",
        },
        message: "make it level two",
        platform: "zhihu",
        title: "Draft",
      }),
    ).resolves.toBe("## Draft");

    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/user/dashboard/ai/prepublish/edit/stream");
    expect(init?.body).toBe(
      JSON.stringify({
        adapted_content: {
          format: "markdown",
          markdown: "# Draft",
        },
        message: "make it level two",
        platform: "zhihu",
        title: "Draft",
      }),
    );
  });

  it("waits for queued publications to reach a final state", async () => {
    const publishing = {
      items: [
        {
          adapted_content: {},
          config: {},
          created_at: "2026-05-29T12:00:00Z",
          enabled: true,
          id: "pub-1",
          platform: "wechat",
          retry_count: 0,
          status: "publishing",
          updated_at: "2026-05-29T12:00:00Z",
        },
      ],
      project_id: "project-1",
    } as ProjectPublications;
    const published = {
      items: [
        {
          adapted_content: {},
          config: {},
          created_at: "2026-05-29T12:00:00Z",
          enabled: true,
          id: "pub-1",
          platform: "wechat",
          publish_url: "https://example.com/post",
          retry_count: 0,
          status: "published",
          updated_at: "2026-05-29T12:00:00Z",
        },
      ],
      project_id: "project-1",
    } as ProjectPublications;

    const fetchProjectPublications = vi
      .fn()
      .mockResolvedValueOnce(publishing)
      .mockResolvedValueOnce(published);

    await expect(
      waitForProjectPublications("project-1", ["wechat"], {
        fetchProjectPublications,
        sleep: async () => {},
      }),
    ).resolves.toEqual(published);

    expect(fetchProjectPublications).toHaveBeenCalledTimes(2);
  });

  it("creates a project with selected platforms", async () => {
    const project = {
      created_at: "2026-05-29T12:00:00Z",
      id: "project-1",
      publications: [
        {
          enabled: true,
          id: "pub-1",
          platform: "wechat",
          status: "adapted",
        },
      ],
      status: "ready",
      title: "New post",
      updated_at: "2026-05-29T12:00:00Z",
      user_id: "user-1",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(project));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      createDashboardProject({
        cover_image_url: "data:image/png;base64,aGVsbG8=",
        platforms: ["wechat"],
        source_content: "<p>Body</p>",
        summary: "Body",
        title: "New post",
      }),
    ).resolves.toEqual(project);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects",
      expect.objectContaining({
        body: JSON.stringify({
          cover_image_url: "data:image/png;base64,aGVsbG8=",
          platforms: ["wechat"],
          source_content: "<p>Body</p>",
          summary: "Body",
          title: "New post",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "POST",
      }),
    );
  });

  it("fetches a project detail for editing", async () => {
    const project = {
      created_at: "2026-05-29T12:00:00Z",
      id: "project-1",
      publications: [
        {
          enabled: true,
          id: "pub-1",
          platform: "wechat",
          status: "published",
        },
      ],
      source_content: "<p>Body</p>",
      status: "ready",
      title: "Existing post",
      updated_at: "2026-05-29T12:00:00Z",
      user_id: "user-1",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(project));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getDashboardProject("project-1")).resolves.toEqual(project);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
  });

  it("updates a project with edited content and selected platforms", async () => {
    const project = {
      created_at: "2026-05-29T12:00:00Z",
      id: "project-1",
      publications: [],
      source_content: "<p>Updated</p>",
      status: "ready",
      title: "Updated post",
      updated_at: "2026-05-29T12:00:00Z",
      user_id: "user-1",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(project));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      updateDashboardProject("project-1", {
        platforms: ["wechat", "zhihu"],
        source_content: "<p>Updated</p>",
        summary: "Updated",
        title: "Updated post",
      }),
    ).resolves.toEqual(project);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1",
      expect.objectContaining({
        body: JSON.stringify({
          platforms: ["wechat", "zhihu"],
          source_content: "<p>Updated</p>",
          summary: "Updated",
          title: "Updated post",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "PUT",
      }),
    );
  });

  it("posts a publish request with the selected platform", async () => {
    const result = {
      publish_url: "https://example.com/post",
      status: "published",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(result));
    vi.stubGlobal("fetch", fetchMock);

    await expect(publishProject("project-1", "wechat")).resolves.toEqual(
      result,
    );

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1/publish",
      expect.objectContaining({
        body: JSON.stringify({ platform: "wechat" }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "POST",
      }),
    );
    const [, init] = fetchMock.mock.calls[0];
    const headers = init!.headers as Headers;
    expect(headers.get("Content-Type")).toBe("application/json");
  });

  it("posts a manual publish request when requested", async () => {
    const result = {
      publish_url: "https://x.com/intent/post?text=hello",
      status: "manual_required",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(result));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      publishProject("project-1", "x", { mode: "manual" }),
    ).resolves.toEqual(result);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1/publish",
      expect.objectContaining({
        body: JSON.stringify({ mode: "manual", platform: "x" }),
        method: "POST",
      }),
    );
  });

  it("fetches and updates the WeChat account settings", async () => {
    const account = {
      account_auth: {
        message: "需要确认公众号认证",
        status: "unknown",
        title: "无法自动确认",
      },
      app_id: "wx-app",
      has_app_secret: true,
      ip_whitelist: {
        message: "等待测试",
        status: "unknown",
        title: "等待测试",
      },
      platform: "wechat",
      status: "untested",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(account));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getWechatAccount()).resolves.toEqual(account);
    await expect(
      saveWechatAccount({ app_id: "wx-app", app_secret: "wx-secret" }),
    ).resolves.toEqual(account);

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/user/dashboard/settings/wechat/account",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/user/dashboard/settings/wechat/account",
      expect.objectContaining({
        body: JSON.stringify({
          app_id: "wx-app",
          app_secret: "wx-secret",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "PUT",
      }),
    );
  });

  it("posts WeChat connection test credentials", async () => {
    const result = {
      account_auth: {
        message: "连接成功不等于具备发布权限",
        status: "warning",
        title: "需确认认证与发布权限",
      },
      connected: true,
      ip_whitelist: {
        message: "微信接口已接受当前服务器请求",
        status: "passed",
        title: "IP 白名单已通过",
      },
      message: "连接成功",
      status: "connected",
      tested_at: "2026-05-29T12:00:00Z",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(result));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      testWechatConnection({ app_id: "wx-app", app_secret: "wx-secret" }),
    ).resolves.toEqual(result);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/settings/wechat/test",
      expect.objectContaining({
        body: JSON.stringify({
          app_id: "wx-app",
          app_secret: "wx-secret",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "POST",
      }),
    );
  });

  it("fetches and updates the X account settings", async () => {
    const account = {
      account_auth: {
        message: "账号凭证已通过",
        status: "passed",
        title: "账号凭证已通过",
      },
      api_key: "x-api-key",
      has_access_token: true,
      has_access_token_secret: true,
      has_api_secret: true,
      platform: "x",
      publish_access: {
        message: "发布前请确认 X App 开启 Read and write 用户权限。",
        status: "unknown",
        title: "等待测试",
      },
      status: "untested",
      username: "creator",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(account));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getXAccount()).resolves.toEqual(account);
    await expect(
      saveXAccount({
        access_token: "x-access-token",
        access_token_secret: "x-access-secret",
        api_key: "x-api-key",
        api_secret: "x-api-secret",
        username: "creator",
      }),
    ).resolves.toEqual(account);

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/user/dashboard/settings/x/account",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/user/dashboard/settings/x/account",
      expect.objectContaining({
        body: JSON.stringify({
          access_token: "x-access-token",
          access_token_secret: "x-access-secret",
          api_key: "x-api-key",
          api_secret: "x-api-secret",
          username: "creator",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "PUT",
      }),
    );
  });

  it("posts X connection test credentials", async () => {
    const result = {
      account_auth: {
        message: "已连接 @creator。",
        status: "passed",
        title: "账号凭证已通过",
      },
      connected: true,
      message: "连接成功",
      name: "Creator",
      publish_access: {
        message:
          "测试会校验账号身份；实际发布还要求 X App 具备 Read and write 权限。",
        status: "warning",
        title: "需确认写入权限",
      },
      status: "connected",
      tested_at: "2026-05-29T12:00:00Z",
      user_id: "123",
      username: "creator",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(result));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      testXConnection({
        access_token: "x-access-token",
        access_token_secret: "x-access-secret",
        api_key: "x-api-key",
        api_secret: "x-api-secret",
      }),
    ).resolves.toEqual(result);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/settings/x/test",
      expect.objectContaining({
        body: JSON.stringify({
          access_token: "x-access-token",
          access_token_secret: "x-access-secret",
          api_key: "x-api-key",
          api_secret: "x-api-secret",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "POST",
      }),
    );
  });

  it("fetches Douyin account status", async () => {
    const account = {
      platform: "douyin",
      status: "connected",
      updated_at: "2026-05-31T12:00:00Z",
      username: "creator",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(account));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getDouyinAccount()).resolves.toEqual(account);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/settings/douyin/account",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
  });

  it("controls remote browser sessions", async () => {
    const start = {
      expires_at: "2026-05-31T12:15:00Z",
      session_id: "session-1",
      status: "ready",
      stream_token_expires_at: "2026-05-31T12:05:00Z",
      stream_url:
        "/api/user/dashboard/browser-sessions/session-1/stream?token=t",
    };
    const session = {
      ...start,
      platform: "douyin",
    };
    const complete = {
      account: { avatar_url: "", username: "creator" },
      message: "Connected",
      platform: "douyin",
      session_id: "session-1",
      status: "connected",
    };
    const cancel = {
      session_id: "session-1",
      status: "expired",
    };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(start))
      .mockResolvedValueOnce(jsonResponse(session))
      .mockResolvedValueOnce(jsonResponse(complete))
      .mockResolvedValueOnce(jsonResponse(cancel));
    vi.stubGlobal("fetch", fetchMock);

    await expect(startBrowserSession("douyin")).resolves.toEqual(start);
    await expect(getBrowserSession("session-1")).resolves.toEqual(session);
    await expect(completeBrowserSession("session-1")).resolves.toEqual(
      complete,
    );
    await expect(cancelBrowserSession("session-1")).resolves.toEqual(cancel);

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/user/dashboard/settings/platforms/douyin/browser-session",
      expect.objectContaining({ method: "POST" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/user/dashboard/browser-sessions/session-1",
      expect.objectContaining({ credentials: "same-origin" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "/api/user/dashboard/browser-sessions/session-1/complete",
      expect.objectContaining({ method: "POST" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      4,
      "/api/user/dashboard/browser-sessions/session-1",
      expect.objectContaining({ method: "DELETE" }),
    );
  });

  it("falls back to the HTTP status when an error response is not JSON", async () => {
    const fetchMock = vi.fn<typeof fetch>(
      async () => new Response("service unavailable", { status: 503 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(getDashboardStats()).rejects.toThrow("请求失败 (503)");
  });
});

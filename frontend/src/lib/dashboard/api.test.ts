// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  acceptProjectShareLink,
  addProjectCollaborator,
  addWorkspaceMember,
  cancelBrowserSession,
  completeBrowserSession,
  completeMediaUpload,
  createBrandProfile,
  createContentTemplate,
  createDashboardProject,
  createProjectMediaUpload,
  createProjectComment,
  createProjectCollabSession,
  createProjectShareLink,
  createWorkspace,
  createWorkspaceProject,
  getBrowserSession,
  getBrandProfiles,
  getContentTemplates,
  getDashboardProject,
  getDashboardProjects,
  getDashboardStats,
  getDouyinAccount,
  getProjectActivities,
  getProjectCollaborators,
  getProjectComments,
  getProjectPublications,
  getProjectShareLinks,
  getProjectVersions,
  getWorkspace,
  getWorkspaceActivities,
  getWorkspaceBrandProfiles,
  getWorkspaceContentTemplates,
  getWorkspaceMembers,
  getWorkspaceProjects,
  getWorkspaces,
  getXAccount,
  getWechatAccount,
  publishProject,
  removeProjectCollaborator,
  resolveMediaAssets,
  removeWorkspaceMember,
  restoreProjectVersion,
  revokeProjectShareLink,
  saveDashboardProjectContent,
  saveDashboardProjectPlatforms,
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
  updateProjectComment,
  updateProjectCollaborator,
  updateProjectPrepublishDraft,
  updateWorkspace,
  updateWorkspaceMember,
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
    expect(path).toBe("/api/user/dashboard/projects?page=1&limit=12");
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
          status: "draft",
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
          status: "draft",
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

  it("surfaces empty AI content streams as errors", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => textStreamResponse([]));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      streamAIContentEdit({
        content: "",
        message: "write a hello world example",
        title: "Draft",
      }),
    ).rejects.toThrow("AI returned no content");
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

  it("updates a platform prepublish draft", async () => {
    const publications = {
      items: [
        {
          adapted_content: {
            format: "markdown",
            markdown: "## Updated",
          },
          config: {},
          created_at: "2026-05-29T12:00:00Z",
          enabled: true,
          id: "pub-1",
          platform: "zhihu",
          retry_count: 0,
          status: "draft",
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
      updateProjectPrepublishDraft("project-1", "zhihu", {
        adapted_content: {
          format: "markdown",
          markdown: "## Updated",
        },
      }),
    ).resolves.toEqual(publications);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1/prepublish/zhihu",
      expect.objectContaining({
        body: JSON.stringify({
          adapted_content: {
            format: "markdown",
            markdown: "## Updated",
          },
        }),
        method: "PUT",
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
          status: "succeeded",
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
          status: "draft",
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

  it("lists and creates content templates and brand profiles", async () => {
    const templates = {
      items: [
        {
          created_at: "2026-06-06T12:00:00Z",
          default_platforms: ["wechat"],
          description: "Launch content",
          id: "template-1",
          name: "Launch",
          platform_config: {},
          scope: "workspace",
          source_template: "<p>Body</p>",
          tags: ["launch"],
          title_template: "Launch title",
          updated_at: "2026-06-06T12:00:00Z",
          workspace_id: "workspace-1",
        },
      ],
    };
    const template = templates.items[0];
    const profiles = {
      items: [
        {
          audience: "Founders",
          banned_words: [],
          created_at: "2026-06-06T12:00:00Z",
          created_by: "user-1",
          cta: "Book a demo",
          default_tags: ["saas"],
          id: "brand-1",
          link_strategy: "Primary CTA",
          name: "MPP",
          updated_at: "2026-06-06T12:00:00Z",
          voice: "Clear",
          workspace_id: "workspace-1",
        },
      ],
    };
    const profile = profiles.items[0];
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(templates))
      .mockResolvedValueOnce(jsonResponse(template, { status: 201 }))
      .mockResolvedValueOnce(jsonResponse(profiles))
      .mockResolvedValueOnce(jsonResponse(profile, { status: 201 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getContentTemplates()).resolves.toEqual(templates);
    await expect(
      createContentTemplate({
        default_platforms: ["wechat"],
        name: "Launch",
        source_template: "<p>Body</p>",
        title_template: "Launch title",
      }),
    ).resolves.toEqual(template);
    await expect(getBrandProfiles()).resolves.toEqual(profiles);
    await expect(createBrandProfile({ name: "MPP" })).resolves.toEqual(
      profile,
    );

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/user/dashboard/content-templates",
      expect.objectContaining({ credentials: "same-origin" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/user/dashboard/content-templates",
      expect.objectContaining({
        body: JSON.stringify({
          default_platforms: ["wechat"],
          name: "Launch",
          source_template: "<p>Body</p>",
          title_template: "Launch title",
        }),
        method: "POST",
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "/api/user/dashboard/brand-profiles",
      expect.objectContaining({ credentials: "same-origin" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      4,
      "/api/user/dashboard/brand-profiles",
      expect.objectContaining({
        body: JSON.stringify({ name: "MPP" }),
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
          status: "succeeded",
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

  it("saves project content without touching selected platforms", async () => {
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
      saveDashboardProjectContent("project-1", {
        source_content: "<p>Updated</p>",
        summary: "Updated",
        title: "Updated post",
      }),
    ).resolves.toEqual(project);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1/content",
      expect.objectContaining({
        body: JSON.stringify({
          source_content: "<p>Updated</p>",
          summary: "Updated",
          title: "Updated post",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "PATCH",
      }),
    );
  });

  it("saves project platform selections without touching content", async () => {
    const project = {
      created_at: "2026-05-29T12:00:00Z",
      id: "project-1",
      publications: [
        {
          enabled: true,
          id: "pub-1",
          platform: "zhihu",
          status: "draft",
        },
      ],
      source_content: "<p>Updated</p>",
      status: "ready",
      title: "Updated post",
      updated_at: "2026-05-29T12:00:00Z",
      user_id: "user-1",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(project));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      saveDashboardProjectPlatforms("project-1", {
        platforms: ["zhihu"],
      }),
    ).resolves.toEqual(project);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1/platforms",
      expect.objectContaining({
        body: JSON.stringify({
          platforms: ["zhihu"],
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "PATCH",
      }),
    );
  });

  it("creates, completes, and resolves project media uploads", async () => {
    const upload = {
      asset_id: "asset-1",
      expires_at: "2026-06-05T12:10:00Z",
      headers: {
        "Content-Type": "image/png",
      },
      object_ref: "mpp://media/asset-1",
      upload_url: "https://r2.example/upload",
    };
    const complete = {
      asset_id: "asset-1",
      object_ref: "mpp://media/asset-1",
      status: "ready",
    };
    const resolved = {
      items: [
        {
          asset_id: "asset-1",
          expires_at: "2026-06-05T12:05:00Z",
          url: "https://r2.example/download",
        },
      ],
    };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(upload, { status: 201 }))
      .mockResolvedValueOnce(jsonResponse(complete))
      .mockResolvedValueOnce(jsonResponse(resolved));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      createProjectMediaUpload("project-1", {
        alt_text: "Cover image",
        filename: "cover.png",
        library_scope: "workspace",
        mime_type: "image/png",
        size_bytes: 1024,
        source: "upload",
        tags: ["launch"],
        usage: "editor_image",
      }),
    ).resolves.toEqual(upload);
    await expect(completeMediaUpload("asset-1")).resolves.toEqual(complete);
    await expect(resolveMediaAssets(["asset-1"])).resolves.toEqual(resolved);

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/user/dashboard/projects/project-1/media/uploads",
      expect.objectContaining({
        body: JSON.stringify({
          alt_text: "Cover image",
          filename: "cover.png",
          library_scope: "workspace",
          mime_type: "image/png",
          size_bytes: 1024,
          source: "upload",
          tags: ["launch"],
          usage: "editor_image",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "POST",
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/user/dashboard/media/asset-1/complete",
      expect.objectContaining({
        method: "POST",
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "/api/user/dashboard/media/resolve",
      expect.objectContaining({
        body: JSON.stringify({
          asset_ids: ["asset-1"],
        }),
        method: "POST",
      }),
    );
  });

  it("lists project collaborators", async () => {
    const collaborators = {
      items: [
        {
          created_at: "2026-06-04T12:00:00Z",
          created_by: "owner-1",
          email: "editor@example.com",
          project_id: "project-1",
          role: "editor",
          user_id: "user-2",
          username: "editor",
        },
      ],
    };
    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse(collaborators),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(getProjectCollaborators("project-1")).resolves.toEqual(
      collaborators,
    );

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1/collaborators",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
  });

  it("adds a project collaborator", async () => {
    const collaborator = {
      created_at: "2026-06-04T12:00:00Z",
      created_by: "owner-1",
      email: "editor@example.com",
      project_id: "project-1",
      role: "editor",
      user_id: "user-2",
      username: "editor",
    };
    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse(collaborator),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      addProjectCollaborator("project-1", {
        email: "editor@example.com",
        role: "editor",
      }),
    ).resolves.toEqual(collaborator);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1/collaborators",
      expect.objectContaining({
        body: JSON.stringify({
          email: "editor@example.com",
          role: "editor",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "POST",
      }),
    );
  });

  it("updates a project collaborator role", async () => {
    const collaborator = {
      created_at: "2026-06-04T12:00:00Z",
      created_by: "owner-1",
      email: "viewer@example.com",
      project_id: "project-1",
      role: "viewer",
      user_id: "user-2",
      username: "viewer",
    };
    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse(collaborator),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      updateProjectCollaborator("project-1", "user-2", { role: "viewer" }),
    ).resolves.toEqual(collaborator);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1/collaborators/user-2",
      expect.objectContaining({
        body: JSON.stringify({ role: "viewer" }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "PATCH",
      }),
    );
  });

  it("removes a project collaborator without parsing an empty response", async () => {
    const fetchMock = vi.fn<typeof fetch>(
      async () => new Response(null, { status: 204 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      removeProjectCollaborator("project-1", "user-2"),
    ).resolves.toBeUndefined();

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1/collaborators/user-2",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "DELETE",
      }),
    );
  });

  it("lists workspaces", async () => {
    const workspaces = {
      items: [
        {
          created_at: "2026-06-05T12:00:00Z",
          id: "workspace-1",
          name: "Team Workspace",
          owner_user_id: "owner-1",
          role: "owner",
          slug: "team-workspace",
          status: "active",
          updated_at: "2026-06-05T12:00:00Z",
        },
      ],
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(workspaces));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getWorkspaces()).resolves.toEqual(workspaces);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/workspaces",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
  });

  it("creates a workspace", async () => {
    const workspace = {
      created_at: "2026-06-05T12:00:00Z",
      id: "workspace-1",
      name: "Team Workspace",
      owner_user_id: "owner-1",
      role: "owner",
      slug: "team-workspace",
      status: "active",
      updated_at: "2026-06-05T12:00:00Z",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(workspace));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      createWorkspace({ name: "Team Workspace", slug: "team-workspace" }),
    ).resolves.toEqual(workspace);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/workspaces",
      expect.objectContaining({
        body: JSON.stringify({
          name: "Team Workspace",
          slug: "team-workspace",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "POST",
      }),
    );
  });

  it("gets and updates a workspace", async () => {
    const workspace = {
      created_at: "2026-06-05T12:00:00Z",
      id: "workspace-1",
      name: "Renamed Workspace",
      owner_user_id: "owner-1",
      role: "admin",
      slug: "renamed-workspace",
      status: "active",
      updated_at: "2026-06-05T12:00:00Z",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(workspace));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getWorkspace("workspace-1")).resolves.toEqual(workspace);
    await expect(
      updateWorkspace("workspace-1", {
        name: "Renamed Workspace",
        slug: "renamed-workspace",
      }),
    ).resolves.toEqual(workspace);

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/workspaces/workspace-1",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/workspaces/workspace-1",
      expect.objectContaining({
        body: JSON.stringify({
          name: "Renamed Workspace",
          slug: "renamed-workspace",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "PATCH",
      }),
    );
  });

  it("manages workspace members", async () => {
    const members = {
      items: [
        {
          created_at: "2026-06-05T12:00:00Z",
          email: "member@example.com",
          role: "member",
          user_id: "user-2",
          username: "member",
          workspace_id: "workspace-1",
        },
      ],
    };
    const member = {
      created_at: "2026-06-05T12:00:00Z",
      email: "member@example.com",
      invited_by: "owner-1",
      joined_at: "2026-06-05T12:00:00Z",
      role: "viewer",
      user_id: "user-2",
      username: "member",
      workspace_id: "workspace-1",
    };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(members))
      .mockResolvedValueOnce(jsonResponse(member))
      .mockResolvedValueOnce(jsonResponse(member))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getWorkspaceMembers("workspace-1")).resolves.toEqual(members);
    await expect(
      addWorkspaceMember("workspace-1", {
        email: "member@example.com",
        role: "member",
      }),
    ).resolves.toEqual(member);
    await expect(
      updateWorkspaceMember("workspace-1", "user-2", { role: "viewer" }),
    ).resolves.toEqual(member);
    await expect(
      removeWorkspaceMember("workspace-1", "user-2"),
    ).resolves.toBeUndefined();

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/workspaces/workspace-1/members",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/workspaces/workspace-1/members",
      expect.objectContaining({
        body: JSON.stringify({
          email: "member@example.com",
          role: "member",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "POST",
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "/api/workspaces/workspace-1/members/user-2",
      expect.objectContaining({
        body: JSON.stringify({ role: "viewer" }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "PATCH",
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      4,
      "/api/workspaces/workspace-1/members/user-2",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "DELETE",
      }),
    );
  });

  it("lists workspace activity", async () => {
    const activities = {
      items: [
        {
          actor_email: "owner@example.com",
          actor_user_id: "owner-1",
          actor_username: "owner",
          created_at: "2026-06-05T12:00:00Z",
          event_type: "member_added",
          id: "activity-1",
          metadata: { role: "member" },
          target_email: "member@example.com",
          target_user_id: "user-2",
          target_username: "member",
          workspace_id: "workspace-1",
        },
      ],
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(activities));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getWorkspaceActivities("workspace-1", 5)).resolves.toEqual(
      activities,
    );

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/workspaces/workspace-1/activity?limit=5",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
  });

  it("lists and creates workspace projects", async () => {
    const projects = {
      items: [
        {
          created_at: "2026-06-05T12:00:00Z",
          id: "project-1",
          publications: [],
          role: "editor",
          status: "ready",
          title: "Team Project",
          updated_at: "2026-06-05T12:00:00Z",
          user_id: "user-1",
          workspace_id: "workspace-1",
        },
      ],
      limit: 20,
      page: 2,
      total: 1,
      total_pages: 1,
    };
    const project = projects.items[0];
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(projects))
      .mockResolvedValueOnce(jsonResponse(project));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      getWorkspaceProjects("workspace-1", {
        limit: 20,
        page: 2,
        platform: "wechat",
        status: "ready",
      }),
    ).resolves.toEqual(projects);
    await expect(
      createWorkspaceProject("workspace-1", {
        platforms: ["wechat"],
        source_content: "<p>team</p>",
        title: "Team Project",
      }),
    ).resolves.toEqual(project);

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/workspaces/workspace-1/projects?page=2&limit=20&status=ready&platform=wechat",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/workspaces/workspace-1/projects",
      expect.objectContaining({
        body: JSON.stringify({
          platforms: ["wechat"],
          source_content: "<p>team</p>",
          title: "Team Project",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "POST",
      }),
    );
  });

  it("lists workspace content templates and brand profiles", async () => {
    const templates = {
      items: [
        {
          created_at: "2026-06-06T12:00:00Z",
          default_platforms: ["wechat"],
          description: "Workspace launch",
          id: "template-1",
          name: "Workspace template",
          platform_config: {},
          scope: "workspace",
          source_template: "<p>Workspace body</p>",
          tags: [],
          title_template: "Workspace title",
          updated_at: "2026-06-06T12:00:00Z",
          workspace_id: "workspace-1",
        },
      ],
    };
    const profiles = {
      items: [
        {
          audience: "Teams",
          banned_words: [],
          created_at: "2026-06-06T12:00:00Z",
          created_by: "user-1",
          cta: "",
          default_tags: [],
          id: "brand-1",
          link_strategy: "",
          name: "Workspace brand",
          updated_at: "2026-06-06T12:00:00Z",
          voice: "Focused",
          workspace_id: "workspace-1",
        },
      ],
    };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(templates))
      .mockResolvedValueOnce(jsonResponse(profiles));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getWorkspaceContentTemplates("workspace-1")).resolves.toEqual(
      templates,
    );
    await expect(getWorkspaceBrandProfiles("workspace-1")).resolves.toEqual(
      profiles,
    );

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/workspaces/workspace-1/content-templates",
      expect.objectContaining({ credentials: "same-origin" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/workspaces/workspace-1/brand-profiles",
      expect.objectContaining({ credentials: "same-origin" }),
    );
  });

  it("creates a project collaboration session", async () => {
    const session = {
      document_id: "document-1",
      expires_at: "2026-06-05T12:00:00Z",
      limits: {
        heartbeat_seconds: 30,
        max_message_bytes: 524288,
      },
      role: "editor",
      token: "collab-token",
      websocket_url: "ws://collab.test/collab/documents/document-1",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(session));
    vi.stubGlobal("fetch", fetchMock);

    await expect(createProjectCollabSession("project-1")).resolves.toEqual(
      session,
    );

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1/collab/session",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "POST",
      }),
    );
  });

  it("uses project collaboration experience endpoints", async () => {
    const activities = {
      items: [
        {
          actor_email: "owner@example.com",
          actor_user_id: "owner-1",
          actor_username: "owner",
          created_at: "2026-06-05T12:00:00Z",
          event_type: "content_saved",
          id: "activity-1",
          metadata: {},
          project_id: "project-1",
        },
      ],
    };
    const comments = {
      items: [
        {
          author_email: "reviewer@example.com",
          author_id: "reviewer-1",
          author_username: "reviewer",
          body: "Needs a clearer intro",
          created_at: "2026-06-05T12:00:00Z",
          id: "comment-1",
          metadata: {},
          project_id: "project-1",
          status: "open",
        },
      ],
    };
    const comment = { ...comments.items[0], status: "resolved" };
    const versions = {
      items: [
        {
          collab_seq: 4,
          created_at: "2026-06-05T12:00:00Z",
          created_by: "owner-1",
          creator_email: "owner@example.com",
          creator_username: "owner",
          id: "version-1",
          project_id: "project-1",
          source: "content_save",
          title: "Draft",
          version_number: 1,
        },
      ],
    };
    const restored = {
      project: {
        created_at: "2026-06-05T12:00:00Z",
        id: "project-1",
        publications: [],
        role: "owner",
        source_content: "<p>Restored</p>",
        status: "ready",
        title: "Restored",
        updated_at: "2026-06-05T12:00:00Z",
        user_id: "owner-1",
      },
      version: versions.items[0],
    };
    const links = {
      items: [
        {
          created_at: "2026-06-05T12:00:00Z",
          created_by: "owner-1",
          id: "link-1",
          project_id: "project-1",
          role: "viewer",
          status: "active",
        },
      ],
    };
    const link = {
      ...links.items[0],
      token: "share-token",
      url: "https://app.example.com/share/projects/share-token",
    };
    const acceptedLink = {
      project: restored.project,
      role: "viewer",
    };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(activities))
      .mockResolvedValueOnce(jsonResponse(comments))
      .mockResolvedValueOnce(jsonResponse(comments.items[0]))
      .mockResolvedValueOnce(jsonResponse(comment))
      .mockResolvedValueOnce(jsonResponse(versions))
      .mockResolvedValueOnce(jsonResponse(restored))
      .mockResolvedValueOnce(jsonResponse(links))
      .mockResolvedValueOnce(jsonResponse(link))
      .mockResolvedValueOnce(jsonResponse(acceptedLink))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getProjectActivities("project-1", 25)).resolves.toEqual(
      activities,
    );
    await expect(getProjectComments("project-1")).resolves.toEqual(comments);
    await expect(
      createProjectComment("project-1", { body: "Needs a clearer intro" }),
    ).resolves.toEqual(comments.items[0]);
    await expect(
      updateProjectComment("project-1", "comment-1", { status: "resolved" }),
    ).resolves.toEqual(comment);
    await expect(getProjectVersions("project-1")).resolves.toEqual(versions);
    await expect(
      restoreProjectVersion("project-1", "version-1"),
    ).resolves.toEqual(restored);
    await expect(getProjectShareLinks("project-1")).resolves.toEqual(links);
    await expect(
      createProjectShareLink("project-1", { role: "viewer" }),
    ).resolves.toEqual(link);
    await expect(acceptProjectShareLink("share-token")).resolves.toEqual(
      acceptedLink,
    );
    await expect(
      revokeProjectShareLink("project-1", "link-1"),
    ).resolves.toBeUndefined();

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/user/dashboard/projects/project-1/activity?limit=25",
      expect.objectContaining({ credentials: "same-origin" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "/api/user/dashboard/projects/project-1/comments",
      expect.objectContaining({
        body: JSON.stringify({ body: "Needs a clearer intro" }),
        method: "POST",
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      6,
      "/api/user/dashboard/projects/project-1/versions/version-1/restore",
      expect.objectContaining({ method: "POST" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      9,
      "/api/user/dashboard/project-share-links/share-token/accept",
      expect.objectContaining({ method: "POST" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      10,
      "/api/user/dashboard/projects/project-1/share-links/link-1",
      expect.objectContaining({ method: "DELETE" }),
    );
  });

  it("posts a publish request with the selected platform", async () => {
    const result = {
      publish_url: "https://example.com/post",
      status: "succeeded",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(result));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      publishProject("project-1", "wechat", {
        idempotencyKey: "publish-click-1",
      }),
    ).resolves.toEqual(result);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1/publish",
      expect.objectContaining({
        body: JSON.stringify({
          idempotency_key: "publish-click-1",
          platform: "wechat",
        }),
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
      publishProject("project-1", "x", {
        idempotencyKey: "manual-click-1",
        mode: "manual",
      }),
    ).resolves.toEqual(result);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1/publish",
      expect.objectContaining({
        body: JSON.stringify({
          idempotency_key: "manual-click-1",
          mode: "manual",
          platform: "x",
        }),
        method: "POST",
      }),
    );
  });

  it("fetches and updates the WeChat account settings", async () => {
    const account = {
      account_auth: {
        message: "WeChat account verification needs manual confirmation",
        status: "unknown",
        title: "Automatic confirmation unavailable",
      },
      app_id: "wx-app",
      has_app_secret: true,
      ip_whitelist: {
        message: "Waiting for verification",
        status: "unknown",
        title: "Waiting for verification",
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
        message: "Connection success does not guarantee publish permission",
        status: "warning",
        title: "Verify auth and publish permissions",
      },
      connected: true,
      ip_whitelist: {
        message: "The WeChat API accepted the current server request",
        status: "passed",
        title: "IP allowlist verified",
      },
      message: "Connected",
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
        message: "Account credentials verified",
        status: "passed",
        title: "Account credentials verified",
      },
      api_key: "x-api-key",
      has_access_token: true,
      has_access_token_secret: true,
      has_api_secret: true,
      platform: "x",
      publish_access: {
        message:
          "Before publishing, confirm the X App has Read and write user permission.",
        status: "unknown",
        title: "Waiting for verification",
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
        message: "Connected as @creator.",
        status: "passed",
        title: "Account credentials verified",
      },
      connected: true,
      message: "Connected",
      name: "Creator",
      publish_access: {
        message:
          "The test verifies account identity; actual publishing also requires X App Read and write permission.",
        status: "warning",
        title: "Confirm write permission",
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

    await expect(getDashboardStats()).rejects.toThrow("Request failed (503)");
  });
});

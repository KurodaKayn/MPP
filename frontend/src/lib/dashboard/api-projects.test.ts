// @vitest-environment jsdom

import { describe, expect, it, vi } from "vitest";
import {
  addProjectCollaborator,
  completeMediaUpload,
  createBrandProfile,
  createContentTemplate,
  createDashboardProject,
  createProjectMediaUpload,
  deleteDashboardProject,
  getBrandProfiles,
  getContentTemplates,
  getDashboardProject,
  getOwnedProjectCollaboratorSummaries,
  getProjectCollaborators,
  removeProjectCollaborator,
  resolveMediaAssets,
  saveDashboardProjectContent,
  saveDashboardProjectPlatforms,
  updateDashboardProject,
  updateProjectCollaborator,
} from "./api";
import {
  jsonResponse,
  setupDashboardApiTest,
} from "./api-test-utils";

describe("dashboard project api", () => {
  setupDashboardApiTest();

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
    await expect(createBrandProfile({ name: "MPP" })).resolves.toEqual(profile);

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

  it("lists owned project collaborator summaries without selected workspace context", async () => {
    window.localStorage.setItem(
      "mpp.dashboard.selectedWorkspaceId",
      "workspace-1",
    );
    const summaries = {
      items: [
        {
          collaborator_count: 1,
          collaborators: [
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
          project_id: "project-1",
        },
      ],
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(summaries));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getOwnedProjectCollaboratorSummaries()).resolves.toEqual(
      summaries,
    );

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/collaborator-summaries",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
    const headers = fetchMock.mock.calls[0]?.[1]?.headers as Headers;
    expect(headers.get("X-Workspace-ID")).toBeNull();
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

  it("deletes a dashboard project without parsing an empty response", async () => {
    const fetchMock = vi.fn<typeof fetch>(
      async () => new Response(null, { status: 204 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(deleteDashboardProject("project-1")).resolves.toBeUndefined();

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/projects/project-1",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "DELETE",
      }),
    );
  });
});

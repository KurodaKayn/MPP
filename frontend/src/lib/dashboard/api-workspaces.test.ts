// @vitest-environment jsdom

import { describe, expect, it, vi } from "vitest";
import {
  addWorkspaceMember,
  createWorkspace,
  createWorkspaceProject,
  getWorkspace,
  getWorkspaceActivities,
  getWorkspaceBrandProfiles,
  getWorkspaceContentTemplates,
  getWorkspaceMembers,
  getWorkspaceProjects,
  getWorkspaces,
  removeWorkspaceMember,
  updateWorkspace,
  updateWorkspaceMember,
} from "./api";
import {
  jsonResponse,
  setupDashboardApiTest,
} from "./api-test-utils";

describe("dashboard workspace api", () => {
  setupDashboardApiTest();

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
      has_more: true,
      limit: 20,
      page: 2,
      next_cursor: "next-cursor",
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
        cursor: "cursor-1",
        limit: 20,
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
      "/api/workspaces/workspace-1/projects?cursor=cursor-1&limit=20&status=ready&platform=wechat",
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
});

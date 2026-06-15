// @vitest-environment jsdom

import { describe, expect, it, vi } from "vitest";
import {
  acceptProjectShareLink,
  createProjectCollabSession,
  createProjectComment,
  createProjectShareLink,
  getProjectActivities,
  getProjectComments,
  getProjectShareLinks,
  getProjectVersions,
  restoreProjectVersion,
  revokeProjectShareLink,
  updateProjectComment,
} from "./api";
import {
  jsonResponse,
  setupDashboardApiTest,
} from "./api-test-utils";

describe("dashboard project collaboration api", () => {
  setupDashboardApiTest();

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
});

// @vitest-environment jsdom

import { describe, expect, it, vi } from "vitest";
import {
  cancelScheduledPublication,
  getProjectPublications,
  getWorkspacePublicationCalendar,
  publishProject,
  retryScheduledPublication,
  scheduleProjectPublication,
  syncProjectPrepublish,
  updateProjectPrepublishDraft,
  waitForProjectPublications,
} from "./api";
import type { ProjectPublications } from "./api";
import { jsonResponse, setupDashboardApiTest } from "./api-test-utils";

describe("dashboard publication api", () => {
  setupDashboardApiTest();

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

  it("does not cache repeated project publication polls", async () => {
    const publications = {
      items: [],
      project_id: "project-1",
    };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(publications))
      .mockResolvedValueOnce(jsonResponse(publications));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getProjectPublications("project-1")).resolves.toEqual(
      publications,
    );
    await expect(getProjectPublications("project-1")).resolves.toEqual(
      publications,
    );

    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("manages scheduled publications", async () => {
    const schedule = {
      attempts: [],
      created_at: "2026-06-11T08:00:00Z",
      created_by: "user-1",
      id: "schedule-1",
      platform: "wechat",
      project_id: "project-1",
      project_title: "Launch draft",
      publication_id: "pub-1",
      scheduled_at: "2026-06-12T08:00:00Z",
      status: "scheduled",
      timezone: "Asia/Shanghai",
      updated_at: "2026-06-11T08:00:00Z",
      workspace_id: "workspace-1",
    };
    const calendar = { items: [schedule] };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(schedule))
      .mockResolvedValueOnce(jsonResponse(calendar))
      .mockResolvedValueOnce(
        jsonResponse({
          ...schedule,
          cancelled_by: "user-1",
          status: "cancelled",
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          ...schedule,
          attempts: [{ attempt_no: 2, id: "attempt-2", status: "succeeded" }],
          status: "published",
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      scheduleProjectPublication("project-1", {
        idempotency_key: "schedule-key",
        platform: "wechat",
        scheduled_at: "2026-06-12T08:00:00Z",
        timezone: "Asia/Shanghai",
      }),
    ).resolves.toEqual(schedule);
    await expect(
      getWorkspacePublicationCalendar(
        "workspace-1",
        "2026-06-12T00:00:00Z",
        "2026-06-13T00:00:00Z",
      ),
    ).resolves.toEqual(calendar);
    await expect(
      cancelScheduledPublication("project-1", "schedule-1"),
    ).resolves.toEqual({
      ...schedule,
      cancelled_by: "user-1",
      status: "cancelled",
    });
    await expect(
      retryScheduledPublication("project-1", "schedule-1"),
    ).resolves.toEqual({
      ...schedule,
      attempts: [{ attempt_no: 2, id: "attempt-2", status: "succeeded" }],
      status: "published",
    });

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/user/dashboard/projects/project-1/schedules",
      expect.objectContaining({
        body: JSON.stringify({
          idempotency_key: "schedule-key",
          platform: "wechat",
          scheduled_at: "2026-06-12T08:00:00Z",
          timezone: "Asia/Shanghai",
        }),
        method: "POST",
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/workspaces/workspace-1/publication-calendar?from=2026-06-12T00%3A00%3A00Z&to=2026-06-13T00%3A00%3A00Z",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "/api/user/dashboard/projects/project-1/schedules/schedule-1",
      expect.objectContaining({
        method: "DELETE",
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      4,
      "/api/user/dashboard/projects/project-1/schedules/schedule-1/retry",
      expect.objectContaining({
        method: "POST",
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
});

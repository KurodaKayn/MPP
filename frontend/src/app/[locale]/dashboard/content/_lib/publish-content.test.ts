// @vitest-environment jsdom

import { describe, expect, it, vi } from "vitest";
import type { ProjectListItem, ProjectPublications } from "@/lib/dashboard/api";
import {
  publishContentToPlatforms,
  publishExistingProjectToPlatforms,
} from "./publish-content";

const project: ProjectListItem = {
  created_at: "2026-05-29T12:00:00Z",
  id: "project-1",
  publications: [],
  role: "owner",
  status: "ready",
  title: "Post title",
  updated_at: "2026-05-29T12:00:00Z",
  user_id: "user-1",
};

describe("publishContentToPlatforms", () => {
  it("creates a project from editor content before publishing selected platforms", async () => {
    const createProject = vi.fn(async () => project);
    const publishProject = vi.fn(async () => ({
      status: "succeeded" as const,
    }));

    const result = await publishContentToPlatforms(
      {
        content: {
          firstImageSrc: "data:image/png;base64,aGVsbG8=",
          html: "<p>Body</p>",
          text: "Body",
        },
        platforms: ["wechat"],
        title: "Post title",
      },
      {
        createProject,
        publishProject,
      },
    );

    expect(createProject).toHaveBeenCalledWith({
      cover_image_url: "data:image/png;base64,aGVsbG8=",
      platforms: ["wechat"],
      source_content: "<p>Body</p>",
      summary: "Body",
      title: "Post title",
    });
    expect(publishProject).toHaveBeenCalledWith("project-1", "wechat", {
      idempotencyKey: expect.stringMatching(/^project-1:wechat:.+:wechat$/),
    });
    expect(result).toEqual({
      failed: [],
      project,
      succeeded: ["wechat"],
    });
  });

  it("reports failed platform results without dropping successful publishes", async () => {
    const createProject = vi.fn(async () => project);
    const publishProject = vi.fn(
      async (
        projectId: string,
        platform: string,
        options?: { idempotencyKey?: string },
      ) => {
        void projectId;
        void options;
        if (platform === "wechat") {
          return { status: "succeeded" as const };
        }
        return {
          error_message: "no publisher registered",
          status: "failed" as const,
        };
      },
    );

    const result = await publishContentToPlatforms(
      {
        content: {
          firstImageSrc: "",
          html: "<p>Body</p>",
          text: "Body",
        },
        platforms: ["wechat", "douyin"],
        title: "Post title",
      },
      {
        createProject,
        publishProject,
      },
    );

    expect(publishProject).toHaveBeenCalledWith("project-1", "wechat", {
      idempotencyKey: expect.stringMatching(
        /^project-1:wechat,douyin:.+:wechat$/,
      ),
    });
    expect(publishProject).toHaveBeenCalledWith("project-1", "douyin", {
      idempotencyKey: expect.stringMatching(
        /^project-1:wechat,douyin:.+:douyin$/,
      ),
    });
    const wechatKey = publishProject.mock.calls[0][2]?.idempotencyKey;
    const douyinKey = publishProject.mock.calls[1][2]?.idempotencyKey;
    expect(douyinKey).toBe(wechatKey?.replace(/:wechat$/, ":douyin"));
    expect(result.succeeded).toEqual(["wechat"]);
    expect(result.failed).toEqual([
      {
        message: "no publisher registered",
        platform: "douyin",
      },
    ]);
  });

  it("waits for queued publish jobs before reporting success", async () => {
    const createProject = vi.fn(async () => project);
    const publishProject = vi.fn(async () => ({
      job_id: "job-1",
      status: "publishing" as const,
    }));
    const publications = {
      items: [
        {
          adapted_content: { format: "html" },
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
    const waitForProjectPublications = vi.fn(async () => publications);

    const result = await publishContentToPlatforms(
      {
        content: {
          firstImageSrc: "",
          html: "<p>Body</p>",
          text: "Body",
        },
        platforms: ["wechat"],
        title: "Post title",
      },
      {
        createProject,
        publishProject,
        waitForProjectPublications,
      },
    );

    expect(publishProject).toHaveBeenCalledWith("project-1", "wechat", {
      idempotencyKey: expect.stringMatching(/^project-1:wechat:.+:wechat$/),
    });
    expect(waitForProjectPublications).toHaveBeenCalledWith("project-1", [
      "wechat",
    ]);
    expect(result).toEqual({
      failed: [],
      project,
      succeeded: ["wechat"],
    });
  });

  it("uses caller-provided messages for fallback publish failures", async () => {
    const createProject = vi.fn(async () => project);
    const publishProject = vi.fn(async () => ({
      status: "failed" as const,
    }));

    const result = await publishContentToPlatforms(
      {
        content: {
          firstImageSrc: "",
          html: "<p>Body</p>",
          text: "Body",
        },
        platforms: ["wechat"],
        title: "Post title",
      },
      {
        createProject,
        formatPublishFailedMessage: (platform) =>
          `${platform} localized failed`,
        publishProject,
      },
    );

    expect(result.failed).toEqual([
      {
        message: "wechat localized failed",
        platform: "wechat",
      },
    ]);
  });

  it("uses caller-provided messages when queued publication status is missing", async () => {
    const publishProject = vi.fn(async () => ({
      job_id: "job-1",
      status: "publishing" as const,
    }));
    const waitForProjectPublications = vi.fn(
      async () =>
        ({
          items: [],
          project_id: "project-1",
        }) as ProjectPublications,
    );

    const result = await publishExistingProjectToPlatforms(
      {
        attemptKey: "attempt-1",
        platforms: ["wechat"],
        projectId: "project-1",
      },
      {
        formatPublicationMissingMessage: (platform) =>
          `${platform} localized status missing`,
        publishProject,
        waitForProjectPublications,
      },
    );

    expect(result.failed).toEqual([
      {
        message: "wechat localized status missing",
        platform: "wechat",
      },
    ]);
  });
});

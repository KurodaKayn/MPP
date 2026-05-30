// @vitest-environment jsdom

import { describe, expect, it, vi } from "vitest";
import type { ProjectListItem } from "@/lib/dashboard/api";
import { publishContentToPlatforms } from "./publish-content";

const project: ProjectListItem = {
  created_at: "2026-05-29T12:00:00Z",
  id: "project-1",
  publications: [],
  status: "ready",
  title: "Post title",
  updated_at: "2026-05-29T12:00:00Z",
  user_id: "user-1",
};

describe("publishContentToPlatforms", () => {
  it("creates a project from editor content before publishing selected platforms", async () => {
    const createProject = vi.fn(async () => project);
    const publishProject = vi.fn(async () => ({ status: "published" }));

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
    expect(publishProject).toHaveBeenCalledWith("project-1", "wechat");
    expect(result).toEqual({
      failed: [],
      project,
      succeeded: ["wechat"],
    });
  });

  it("reports failed platform results without dropping successful publishes", async () => {
    const createProject = vi.fn(async () => project);
    const publishProject = vi.fn(
      async (projectId: string, platform: string) => {
        if (platform === "wechat") {
          return { status: "published" };
        }
        return { error_message: "no publisher registered", status: "failed" };
      },
    );

    const result = await publishContentToPlatforms(
      {
        content: {
          firstImageSrc: "",
          html: "<p>Body</p>",
          text: "Body",
        },
        platforms: ["wechat", "bilibili"],
        title: "Post title",
      },
      {
        createProject,
        publishProject,
      },
    );

    expect(publishProject).toHaveBeenCalledWith("project-1", "wechat");
    expect(publishProject).toHaveBeenCalledWith("project-1", "bilibili");
    expect(result.succeeded).toEqual(["wechat"]);
    expect(result.failed).toEqual([
      {
        message: "no publisher registered",
        platform: "bilibili",
      },
    ]);
  });
});

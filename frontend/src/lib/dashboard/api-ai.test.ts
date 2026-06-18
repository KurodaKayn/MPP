// @vitest-environment jsdom

import { describe, expect, it, vi } from "vitest";
import {
  applyAIGrowthOptimizationProposal,
  createAIGrowthOptimizationRun,
  streamAIContentEdit,
  streamAIPrepublishEdit,
} from "./api";
import { setupDashboardApiTest, textStreamResponse } from "./api-test-utils";

describe("dashboard ai api", () => {
  setupDashboardApiTest();

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

  it("creates a mock AI growth optimization run while backend endpoints are pending", async () => {
    await expect(
      createAIGrowthOptimizationRun("project-1", {
        goal: "views",
        intensity: "balanced",
        source_content: "Original body",
        target_platforms: ["wechat", "zhihu"],
        title: "Original title",
      }),
    ).resolves.toMatchObject({
      goal: "views",
      intensity: "balanced",
      project_id: "project-1",
      status: "ready",
      target_platforms: ["wechat", "zhihu"],
    });
  });

  it("marks a mock AI growth proposal as accepted", async () => {
    await expect(
      applyAIGrowthOptimizationProposal("project-1", "proposal-wechat"),
    ).resolves.toEqual({
      proposal_id: "proposal-wechat",
      status: "accepted",
    });
  });
});

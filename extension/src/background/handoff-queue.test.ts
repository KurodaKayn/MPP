import { beforeEach, describe, expect, it } from "vitest";
import {
  HANDOFF_SCHEMA_VERSION,
  HANDOFF_TYPE,
  type ExtensionPublishHandoff,
} from "../types/handoff";
import { resetTestStorage } from "../test/wxt-imports";

const {
  clearExecutionState,
  getExecutionQueue,
  isExecutionQueueActive,
  storeAcceptedHandoff,
  updateExecutionQueueTask,
} = await import("./handoff");

function createHandoff(): ExtensionPublishHandoff {
  return {
    schema_version: HANDOFF_SCHEMA_VERSION,
    type: HANDOFF_TYPE,
    execution_id: "execution-1",
    expires_at: "2026-06-03T12:00:00Z",
    project: {
      id: "project-1",
      title: "Multi-platform draft",
    },
    platforms: [
      {
        platform: "douyin",
        adapter_key: "DYNAMIC_DOUYIN",
        inject_url: "https://creator.douyin.com/creator-micro/content/upload",
        content_kind: "article",
        auto_publish: false,
        requires_review: true,
        adapted_content: {
          schema_version: HANDOFF_SCHEMA_VERSION,
          format: "text",
          text: "Douyin draft",
        },
        assets: [],
      },
      {
        platform: "x",
        adapter_key: "POST_X",
        inject_url: "https://x.com/compose/post",
        content_kind: "dynamic_post",
        auto_publish: false,
        requires_review: true,
        adapted_content: {
          schema_version: HANDOFF_SCHEMA_VERSION,
          format: "text",
          text: "X draft",
        },
        assets: [],
      },
    ],
  };
}

describe("execution queue storage", () => {
  beforeEach(() => {
    resetTestStorage();
  });

  it("initializes queued platform tasks when a handoff is accepted", async () => {
    const handoff = createHandoff();

    await storeAcceptedHandoff(handoff, "extension_workbench");

    await expect(getExecutionQueue()).resolves.toMatchObject({
      execution_id: "execution-1",
      project_id: "project-1",
      active_platform: null,
      tasks: [
        {
          platform: "douyin",
          adapter_key: "DYNAMIC_DOUYIN",
          status: "queued",
        },
        {
          platform: "x",
          adapter_key: "POST_X",
          status: "queued",
        },
      ],
    });
    expect(isExecutionQueueActive(await getExecutionQueue())).toBe(true);
  });

  it("updates task status and treats review or failed tasks as inactive", async () => {
    await storeAcceptedHandoff(createHandoff(), "extension_workbench");

    await updateExecutionQueueTask("execution-1", "douyin", {
      status: "injecting",
      tab_id: 10,
    });

    const injectingQueue = await getExecutionQueue();

    expect(injectingQueue?.active_platform).toBe("douyin");
    expect(injectingQueue?.tasks).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          platform: "douyin",
          status: "injecting",
          tab_id: 10,
        }),
      ]),
    );

    await updateExecutionQueueTask("execution-1", "douyin", {
      status: "user_review",
    });
    await updateExecutionQueueTask("execution-1", "x", {
      status: "failed",
      error_message: "X composer missing.",
    });

    const queue = await getExecutionQueue();

    expect(queue).toMatchObject({
      active_platform: null,
      tasks: [
        {
          platform: "douyin",
          status: "user_review",
        },
        {
          platform: "x",
          status: "failed",
          error_message: "X composer missing.",
        },
      ],
    });
    expect(isExecutionQueueActive(queue)).toBe(false);
  });

  it("clears the queue with the rest of execution state", async () => {
    await storeAcceptedHandoff(createHandoff(), "extension_workbench");

    await clearExecutionState();

    await expect(getExecutionQueue()).resolves.toBeNull();
  });
});

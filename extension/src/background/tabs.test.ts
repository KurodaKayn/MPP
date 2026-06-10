import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ExtensionExecutionEvent } from "../types/events";
import type { ExtensionPublishPlatformHandoff } from "../types/handoff";

const callbackMock = vi.hoisted(() => ({
  sanitizeError: vi.fn((error: unknown) =>
    error instanceof Error ? error.message : String(error),
  ),
  sendEventCallback: vi.fn(),
}));

const handoffMock = vi.hoisted(() => ({
  appendStoredExecutionEvent: vi.fn((event: ExtensionExecutionEvent) =>
    Promise.resolve(event),
  ),
  updateExecutionQueueTask: vi.fn(),
}));

vi.mock("./callback", () => callbackMock);
vi.mock("./handoff", () => handoffMock);

const { recordAndCallbackEvent, startPublishingTabs } = await import("./tabs");

function createPlatform(): ExtensionPublishPlatformHandoff {
  return {
    platform: "douyin",
    adapter_key: "DYNAMIC_DOUYIN",
    inject_url: "https://creator.douyin.com/creator-micro/content/upload",
    content_kind: "image_video",
    auto_publish: false,
    requires_review: true,
    adapted_content: {
      schema_version: 1,
      format: "text",
      text: "draft body",
    },
    assets: [],
    callback: {
      url: "https://mpp.example.com/api/extension/events",
      token: "one-time-token",
    },
  };
}

function createXPlatform(): ExtensionPublishPlatformHandoff {
  return {
    platform: "x",
    adapter_key: "POST_X",
    inject_url: "https://x.com/compose/post",
    content_kind: "dynamic_post",
    auto_publish: false,
    requires_review: true,
    adapted_content: {
      schema_version: 1,
      format: "text",
      text: "x draft body",
    },
    assets: [],
  };
}

describe("recordAndCallbackEvent", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(console, "warn").mockImplementation(() => undefined);
  });

  it("stores the same event that was sent to the callback endpoint", async () => {
    callbackMock.sendEventCallback.mockResolvedValue(undefined);

    await recordAndCallbackEvent(createPlatform(), {
      platform: "douyin",
      status: "injecting",
      message: "Injecting platform adapter.",
      metadata: {
        adapter_key: "DYNAMIC_DOUYIN",
      },
    });

    expect(callbackMock.sendEventCallback).toHaveBeenCalledOnce();
    expect(handoffMock.appendStoredExecutionEvent).toHaveBeenCalledOnce();

    const sentEvent = callbackMock.sendEventCallback.mock.calls[0][1];

    expect(sentEvent).toEqual(
      expect.objectContaining({
        event_id: expect.any(String),
        platform: "douyin",
        status: "injecting",
        message: "Injecting platform adapter.",
        remote_id: "",
        publish_url: "",
        error_message: "",
        metadata: {
          adapter_key: "DYNAMIC_DOUYIN",
        },
        created_at: expect.any(String),
      }),
    );
    expect(handoffMock.appendStoredExecutionEvent).toHaveBeenCalledWith(
      sentEvent,
    );
  });

  it("keeps callback failures visible without rejecting adapter execution", async () => {
    callbackMock.sendEventCallback.mockRejectedValue(
      new Error("Callback rejected event with HTTP 500."),
    );

    await expect(
      recordAndCallbackEvent(createPlatform(), {
        platform: "douyin",
        status: "user_review",
        message: "Prepared for user review.",
      }),
    ).resolves.toBeUndefined();

    expect(handoffMock.appendStoredExecutionEvent).toHaveBeenCalledWith(
      expect.objectContaining({
        platform: "douyin",
        status: "user_review",
        metadata: {
          callback_failed: true,
          callback_error: "Callback rejected event with HTTP 500.",
        },
      }),
    );
  });
});

describe("startPublishingTabs", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    callbackMock.sendEventCallback.mockResolvedValue(undefined);
  });

  it("updates queue task status while processing platforms sequentially", async () => {
    const douyin = createPlatform();
    const x = createXPlatform();
    const tabUrls = new Map<number, string>();
    let nextTabId = 0;
    let resolveFirstMessage: (() => void) | undefined;

    vi.stubGlobal("browser", {
      tabs: {
        create: vi.fn(({ url }: { url: string }) => {
          nextTabId += 1;
          tabUrls.set(nextTabId, url);
          return Promise.resolve({ id: nextTabId, url });
        }),
        get: vi.fn((tabId: number) =>
          Promise.resolve({ id: tabId, url: tabUrls.get(tabId) }),
        ),
        onUpdated: {
          addListener: vi.fn(
            (
              listener: (
                tabId: number,
                changeInfo: { status?: string },
              ) => void,
            ) => {
              const tabId = nextTabId;
              queueMicrotask(() => listener(tabId, { status: "complete" }));
            },
          ),
          removeListener: vi.fn(),
        },
        sendMessage: vi.fn((tabId: number) => {
          if (tabId === 1) {
            return new Promise<void>((resolve) => {
              resolveFirstMessage = resolve;
            });
          }

          return Promise.resolve();
        }),
      },
      scripting: {
        executeScript: vi.fn(() => Promise.resolve()),
      },
    });

    const runPromise = startPublishingTabs({
      schema_version: 1,
      type: "mpp.extension_publish_handoff",
      execution_id: "execution-1",
      expires_at: "2026-06-03T12:00:00Z",
      project: {
        id: "project-1",
        title: "Project 1",
      },
      platforms: [douyin, x],
    });

    await vi.waitFor(() => {
      expect(browser.tabs.create).toHaveBeenCalledTimes(1);
    });
    expect(handoffMock.updateExecutionQueueTask).toHaveBeenCalledWith(
      "execution-1",
      "douyin",
      expect.objectContaining({ status: "opening_tabs" }),
    );
    expect(handoffMock.updateExecutionQueueTask).toHaveBeenCalledWith(
      "execution-1",
      "douyin",
      expect.objectContaining({ status: "injecting", tab_id: 1 }),
    );

    resolveFirstMessage?.();
    await runPromise;

    expect(browser.tabs.create).toHaveBeenNthCalledWith(1, {
      active: true,
      url: douyin.inject_url,
    });
    expect(browser.tabs.create).toHaveBeenNthCalledWith(2, {
      active: true,
      url: x.inject_url,
    });
    expect(handoffMock.updateExecutionQueueTask).toHaveBeenCalledWith(
      "execution-1",
      "x",
      expect.objectContaining({ status: "opening_tabs" }),
    );
    expect(handoffMock.updateExecutionQueueTask).toHaveBeenCalledWith(
      "execution-1",
      "x",
      expect.objectContaining({ status: "injecting", tab_id: 2 }),
    );
  });
});

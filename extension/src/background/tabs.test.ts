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
}));

vi.mock("./callback", () => callbackMock);
vi.mock("./handoff", () => handoffMock);

const { recordAndCallbackEvent } = await import("./tabs");

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

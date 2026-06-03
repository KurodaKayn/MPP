import { beforeEach, describe, expect, it, vi } from "vitest";
import { sanitizeError, sendEventCallback } from "./callback";
import type { ExtensionExecutionEvent } from "../types/events";
import type { ExtensionPublishPlatformHandoff } from "../types/handoff";

function createPlatform(
  callback?: ExtensionPublishPlatformHandoff["callback"],
): ExtensionPublishPlatformHandoff {
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
    callback,
  };
}

function createEvent(overrides: Partial<ExtensionExecutionEvent> = {}) {
  return {
    event_id: "event-1",
    platform: "douyin",
    status: "user_review",
    message: "Draft is ready for review.",
    remote_id: "",
    publish_url: "",
    error_message: "",
    metadata: {
      adapter_key: "DYNAMIC_DOUYIN",
    },
    created_at: "2026-01-01T00:00:00.000Z",
    ...overrides,
  } satisfies ExtensionExecutionEvent;
}

describe("sendEventCallback", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it("posts callback events with the one-time token and idempotent event id", async () => {
    const fetchMock = vi.fn((_url: RequestInfo | URL, _request?: RequestInit) =>
      Promise.resolve(
        new Response(null, {
          status: 204,
        }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await sendEventCallback(
      createPlatform({
        url: "https://mpp.example.com/api/extension/events",
        token: "one-time-token",
      }),
      createEvent(),
    );

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock).toHaveBeenCalledWith(
      "https://mpp.example.com/api/extension/events",
      expect.objectContaining({
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
      }),
    );

    const request = fetchMock.mock.calls[0]?.[1];
    if (!request) {
      throw new Error("Expected callback request options.");
    }
    const payload = JSON.parse(String(request.body));

    expect(payload).toEqual({
      token: "one-time-token",
      event_id: "event-1",
      platform: "douyin",
      status: "user_review",
      message: "Draft is ready for review.",
      remote_id: "",
      publish_url: "",
      error_message: "",
      metadata: {
        adapter_key: "DYNAMIC_DOUYIN",
      },
    });
  });

  it("skips network requests when the platform has no callback", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await sendEventCallback(createPlatform(), createEvent());

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("sanitizes callback text fields before sending", async () => {
    const fetchMock = vi.fn((_url: RequestInfo | URL, _request?: RequestInit) =>
      Promise.resolve(
        new Response(null, {
          status: 200,
        }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const longText = "x".repeat(600);

    await sendEventCallback(
      createPlatform({
        url: "https://mpp.example.com/api/extension/events",
        token: "one-time-token",
      }),
      createEvent({
        message: longText,
        remote_id: longText,
        publish_url: longText,
        error_message: longText,
      }),
    );

    const request = fetchMock.mock.calls[0]?.[1];
    if (!request) {
      throw new Error("Expected callback request options.");
    }
    const payload = JSON.parse(String(request.body));

    expect(payload.message).toHaveLength(500);
    expect(payload.remote_id).toHaveLength(500);
    expect(payload.publish_url).toHaveLength(500);
    expect(payload.error_message).toHaveLength(500);
  });

  it("throws a sanitized error when the callback endpoint rejects an event", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(null, {
            status: 429,
          }),
        ),
      ),
    );

    await expect(
      sendEventCallback(
        createPlatform({
          url: "https://mpp.example.com/api/extension/events",
          token: "one-time-token",
        }),
        createEvent(),
      ),
    ).rejects.toThrow("Callback rejected event with HTTP 429.");
  });
});

describe("sanitizeError", () => {
  it("limits error messages sent to callback metadata", () => {
    expect(sanitizeError(new Error("x".repeat(600)))).toHaveLength(500);
    expect(sanitizeError("x".repeat(600))).toHaveLength(500);
  });
});

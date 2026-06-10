import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  HANDOFF_SCHEMA_VERSION,
  HANDOFF_TYPE,
  type ExtensionPublishPlatformHandoff,
  type StoredHandoff,
} from "../types/handoff";
import type {
  ExtensionExecutionEvent,
  PublishExecutionStatus,
} from "../types/events";
import type { BackgroundMessage } from "../types/messages";

const handoffMock = vi.hoisted(() => {
  const state: {
    currentHandoff: StoredHandoff | null;
    events: ExtensionExecutionEvent[];
    executionQueue: unknown;
    expired: boolean;
  } = {
    currentHandoff: null,
    events: [],
    executionQueue: null,
    expired: true,
  };

  return {
    state,
    clearExecutionState: vi.fn(),
    getExecutionQueue: vi.fn(() => Promise.resolve(state.executionQueue)),
    getCurrentHandoff: vi.fn(() => Promise.resolve(state.currentHandoff)),
    getExecutionEvents: vi.fn(() => Promise.resolve(state.events)),
    isExecutionQueueActive: vi.fn((queue: unknown) => queue !== null),
    isExecutionQueueTaskStatus: vi.fn((status: string) =>
      [
        "queued",
        "opening_tabs",
        "injecting",
        "user_review",
        "submitted",
        "succeeded",
        "failed",
        "cancelled",
        "expired",
      ].includes(status),
    ),
    isHandoffExpired: vi.fn(() => state.expired),
    storeAcceptedHandoff: vi.fn(),
    updateExecutionQueueTask: vi.fn(),
    validateHandoff: vi.fn(),
  };
});

const tabsMock = vi.hoisted(() => ({
  recordAndCallbackEvent: vi.fn(),
  startPublishingTabs: vi.fn(),
}));

const originsMock = vi.hoisted(() => {
  const state = {
    trusted: true,
    trustable: true,
  };

  return {
    state,
    getTrustOriginPageUrl: vi.fn((origin: string) => `${origin}/trust`),
    isTrustedOrigin: vi.fn(() => Promise.resolve(state.trusted)),
    isTrustableOrigin: vi.fn(() => state.trustable),
    listTrustedOrigins: vi.fn(() => Promise.resolve([])),
    removeTrustedOrigin: vi.fn(() => Promise.resolve([])),
    trustOrigin: vi.fn((origin: string) => Promise.resolve(origin)),
  };
});

const authMock = vi.hoisted(() => ({
  persistExtensionAuthToken: vi.fn(() => Promise.resolve(false)),
}));

vi.mock("./handoff", () => handoffMock);
vi.mock("./tabs", () => tabsMock);
vi.mock("./origins", () => originsMock);
vi.mock("../backend/auth", () => authMock);

vi.stubGlobal("defineBackground", (callback: () => void) => callback);
vi.stubGlobal("browser", {
  action: {
    onClicked: {
      addListener: vi.fn(),
    },
  },
  runtime: {
    id: "extension-id",
    getManifest: vi.fn(() => ({ version: "1.0.0" })),
    getURL: vi.fn((path: string) => `chrome-extension://extension-id${path}`),
    onMessage: {
      addListener: vi.fn(),
    },
  },
  tabs: {
    create: vi.fn(() => Promise.resolve({ id: 1 })),
  },
});

const {
  acceptExtensionHandoff,
  downloadHandoffAsset,
  handleBackgroundMessage,
  recordCurrentHandoffExpiration,
  shouldRejectExpiredAdapterEvent,
} = await import("../../entrypoints/background");

function createDouyinPlatform(): ExtensionPublishPlatformHandoff {
  return {
    platform: "douyin",
    adapter_key: "DYNAMIC_DOUYIN",
    inject_url: "https://creator.douyin.com/creator-micro/content/upload",
    content_kind: "image_video",
    auto_publish: false,
    requires_review: true,
    adapted_content: {
      schema_version: HANDOFF_SCHEMA_VERSION,
      format: "text",
      text: "draft body",
    },
    assets: [],
  };
}

function createStoredHandoff(): StoredHandoff {
  return {
    handoff: {
      schema_version: HANDOFF_SCHEMA_VERSION,
      type: HANDOFF_TYPE,
      execution_id: "execution-1",
      expires_at: "2026-01-01T00:00:00.000Z",
      project: {
        id: "project-1",
        title: "Project 1",
      },
      platforms: [createDouyinPlatform()],
    },
    accepted_at: "2025-12-31T23:00:00.000Z",
    source_origin: "https://mpp.example.com",
  };
}

function createEvent(status: PublishExecutionStatus): ExtensionExecutionEvent {
  return {
    event_id: `event-${status}`,
    platform: "douyin",
    status,
    message: status,
    remote_id: "",
    publish_url: "",
    error_message: "",
    metadata: {},
    created_at: "2026-01-01T00:00:00.000Z",
  };
}

function createAdapterEventMessage(): Extract<
  BackgroundMessage,
  { type: "adapter.event" }
> {
  return {
    type: "adapter.event",
    execution_id: "execution-1",
    event: {
      platform: "douyin",
      status: "user_review",
      message: "Prepared for review.",
    },
  };
}

describe("background expiration handling", () => {
  beforeEach(() => {
    handoffMock.state.currentHandoff = createStoredHandoff();
    handoffMock.state.events = [];
    handoffMock.state.executionQueue = null;
    handoffMock.state.expired = true;
    originsMock.state.trusted = true;
    originsMock.state.trustable = true;
    vi.clearAllMocks();
  });

  it.each([
    "user_review",
    "submitted",
    "succeeded",
    "failed",
    "cancelled",
    "expired",
  ] satisfies PublishExecutionStatus[])(
    "does not emit expired events after %s",
    async (status) => {
      handoffMock.state.events = [createEvent(status)];

      await expect(recordCurrentHandoffExpiration()).resolves.toBe(false);

      expect(tabsMock.recordAndCallbackEvent).not.toHaveBeenCalled();
    },
  );

  it("records expiration when the platform has not reached review or terminal state", async () => {
    handoffMock.state.events = [createEvent("injecting")];

    await expect(recordCurrentHandoffExpiration()).resolves.toBe(true);

    expect(tabsMock.recordAndCallbackEvent).toHaveBeenCalledWith(
      createDouyinPlatform(),
      expect.objectContaining({
        platform: "douyin",
        status: "expired",
        message: "Handoff expired before extension publishing completed.",
      }),
    );
  });

  it("allows adapter events after expiration when the latest platform event is review-ready", async () => {
    handoffMock.state.events = [createEvent("user_review")];

    await expect(
      shouldRejectExpiredAdapterEvent(createAdapterEventMessage()),
    ).resolves.toBe(false);
  });

  it("rejects adapter events after expiration when no review or terminal event exists", async () => {
    handoffMock.state.events = [createEvent("injecting")];

    await expect(
      shouldRejectExpiredAdapterEvent(createAdapterEventMessage()),
    ).resolves.toBe(true);
  });
});

describe("acceptExtensionHandoff", () => {
  beforeEach(() => {
    handoffMock.state.currentHandoff = null;
    handoffMock.state.events = [];
    handoffMock.state.executionQueue = null;
    handoffMock.state.expired = false;
    originsMock.state.trusted = true;
    originsMock.state.trustable = true;
    vi.clearAllMocks();
  });

  it("stores backend-generated handoffs and starts publishing tabs", async () => {
    const storedHandoff = createStoredHandoff().handoff;
    handoffMock.validateHandoff.mockReturnValue({
      ok: true,
      handoff: storedHandoff,
    });

    await expect(acceptExtensionHandoff({})).resolves.toEqual({
      accepted: true,
      execution_id: storedHandoff.execution_id,
      platforms: storedHandoff.platforms,
    });

    expect(handoffMock.storeAcceptedHandoff).toHaveBeenCalledWith(
      storedHandoff,
      "extension_workbench",
    );
    expect(tabsMock.recordAndCallbackEvent).toHaveBeenCalledWith(
      storedHandoff.platforms[0],
      expect.objectContaining({
        platform: "douyin",
        status: "accepted",
        message: "Extension accepted the publishing handoff.",
      }),
    );
    expect(tabsMock.startPublishingTabs).toHaveBeenCalledWith(storedHandoff);
  });

  it("rejects new handoffs while a local execution queue is active", async () => {
    const storedHandoff = createStoredHandoff().handoff;
    handoffMock.state.executionQueue = {
      execution_id: "active-execution",
      tasks: [
        {
          platform: "douyin",
          status: "injecting",
        },
      ],
    };
    handoffMock.validateHandoff.mockReturnValue({
      ok: true,
      handoff: storedHandoff,
    });

    await expect(acceptExtensionHandoff({})).resolves.toMatchObject({
      accepted: false,
      reason: "active_execution",
      message: "Another extension publishing execution is already active.",
    });

    expect(handoffMock.storeAcceptedHandoff).not.toHaveBeenCalled();
    expect(tabsMock.startPublishingTabs).not.toHaveBeenCalled();
  });
});

describe("bridge compatibility", () => {
  beforeEach(() => {
    handoffMock.state.currentHandoff = createStoredHandoff();
    handoffMock.state.events = [];
    handoffMock.state.executionQueue = null;
    handoffMock.state.expired = false;
    originsMock.state.trusted = true;
    originsMock.state.trustable = true;
    vi.clearAllMocks();
  });

  it("reports detect metadata for trusted bridge origins", async () => {
    await expect(
      handleBackgroundMessage({
        type: "bridge.detect",
        origin: "https://mpp.example.com",
      }),
    ).resolves.toMatchObject({
      installed: true,
      extension_id: "extension-id",
      version: "1.0.0",
      trusted: true,
      trustable: true,
      trust_url: "https://mpp.example.com/trust",
    });
  });

  it("opens the trust page for eligible bridge origins", async () => {
    await expect(
      handleBackgroundMessage({
        type: "bridge.request_trust",
        origin: "https://mpp.example.com",
      }),
    ).resolves.toEqual({
      trust_url: "https://mpp.example.com/trust",
    });

    expect(browser.tabs.create).toHaveBeenCalledWith({
      active: true,
      url: "https://mpp.example.com/trust",
    });
  });

  it("rejects bridge handoffs from untrusted origins before validation", async () => {
    originsMock.state.trusted = false;

    await expect(
      handleBackgroundMessage({
        type: "bridge.publish_handoff",
        origin: "https://mpp.example.com",
        handoff: {},
      }),
    ).resolves.toMatchObject({
      accepted: false,
      reason: "origin_untrusted",
      trust_url: "https://mpp.example.com/trust",
    });

    expect(handoffMock.validateHandoff).not.toHaveBeenCalled();
  });

  it("keeps trusted bridge handoffs tied to their source origin", async () => {
    const storedHandoff = createStoredHandoff().handoff;
    handoffMock.validateHandoff.mockReturnValue({
      ok: true,
      handoff: storedHandoff,
    });

    await expect(
      handleBackgroundMessage({
        type: "bridge.publish_handoff",
        origin: "https://mpp.example.com",
        handoff: {},
      }),
    ).resolves.toMatchObject({
      accepted: true,
      execution_id: "execution-1",
    });

    expect(handoffMock.storeAcceptedHandoff).toHaveBeenCalledWith(
      storedHandoff,
      "https://mpp.example.com",
    );
  });

  it("returns a trust prompt instead of monitor state for untrusted bridge status", async () => {
    originsMock.state.trusted = false;

    await expect(
      handleBackgroundMessage({
        type: "bridge.get_status",
        origin: "https://mpp.example.com",
      }),
    ).resolves.toEqual({
      trusted: false,
      trust_url: "https://mpp.example.com/trust",
    });

    expect(handoffMock.getCurrentHandoff).not.toHaveBeenCalled();
  });
});

describe("extension auth token persistence", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("persists web login tokens forwarded from the page bridge", async () => {
    authMock.persistExtensionAuthToken.mockResolvedValue(true);

    await expect(
      handleBackgroundMessage({
        type: "extension.persist_auth_token",
        token: "Bearer web-token",
      }),
    ).resolves.toEqual({ persisted: true });

    expect(authMock.persistExtensionAuthToken).toHaveBeenCalledWith(
      "Bearer web-token",
    );
  });
});

describe("downloadHandoffAsset", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    vi.stubGlobal("defineBackground", (callback: () => void) => callback);
    vi.stubGlobal("browser", {
      action: {
        onClicked: {
          addListener: vi.fn(),
        },
      },
      runtime: {
        id: "extension-id",
        getManifest: vi.fn(() => ({ version: "1.0.0" })),
        getURL: vi.fn(
          (path: string) => `chrome-extension://extension-id${path}`,
        ),
        onMessage: {
          addListener: vi.fn(),
        },
      },
      tabs: {
        create: vi.fn(() => Promise.resolve({ id: 1 })),
      },
    });
  });

  it("downloads assets from the background with credentials omitted", async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        new Response(new Uint8Array([104, 105]), {
          status: 200,
          headers: {
            "Content-Type": "image/png",
          },
        }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      downloadHandoffAsset({
        type: "image",
        source_url: "https://assets.example.com/image.png",
        name: "image.png",
        mime_type: "image/png",
      }),
    ).resolves.toEqual({
      name: "image.png",
      mime_type: "image/png",
      data_base64: "aGk=",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "https://assets.example.com/image.png",
      {
        credentials: "omit",
      },
    );
  });

  it("rejects invalid asset download requests", async () => {
    await expect(
      downloadHandoffAsset({ source_url: "https://example.com" }),
    ).rejects.toThrow("Invalid handoff asset download request.");
  });
});

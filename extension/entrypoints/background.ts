import {
  clearExecutionState,
  getCurrentHandoff,
  getExecutionEvents,
  isHandoffExpired,
  storeAcceptedHandoff,
  validateHandoff,
} from "../src/background/handoff";
import {
  getTrustOriginPageUrl,
  isTrustedOrigin,
  isTrustableOrigin,
  listTrustedOrigins,
  removeTrustedOrigin,
  trustOrigin,
} from "../src/background/origins";
import {
  recordAndCallbackEvent,
  startPublishingTabs,
} from "../src/background/tabs";
import {
  HANDOFF_SCHEMA_VERSION,
  type HandoffAsset,
  type StoredHandoff,
} from "../src/types/handoff";
import type { ExtensionExecutionEvent } from "../src/types/events";
import type {
  AssetDownloadResponse,
  BackgroundMessage,
  HandoffResponse,
} from "../src/types/messages";
import type { PlatformKey } from "../src/types/platform";

const EXPIRATION_SUPPRESSION_STATUSES = new Set([
  "user_review",
  "submitted",
  "succeeded",
  "failed",
  "cancelled",
  "expired",
]);
const POST_EXPIRATION_EVENT_ALLOWED_STATUSES = new Set([
  "user_review",
  "submitted",
  "succeeded",
  "failed",
  "cancelled",
]);

type CurrentHandoffSnapshot = {
  currentHandoff: StoredHandoff | null;
  events: ExtensionExecutionEvent[];
};

function isBackgroundMessage(value: unknown): value is BackgroundMessage {
  return (
    typeof value === "object" &&
    value !== null &&
    "type" in value &&
    typeof value.type === "string"
  );
}

function getManifestVersion(): string {
  return browser.runtime.getManifest().version ?? "0.0.0";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isHandoffAsset(value: unknown): value is HandoffAsset {
  return (
    isRecord(value) &&
    (value.type === "image" || value.type === "video") &&
    typeof value.source_url === "string" &&
    typeof value.name === "string" &&
    typeof value.mime_type === "string"
  );
}

function arrayBufferToBase64(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = "";

  for (let index = 0; index < bytes.length; index += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(index, index + 0x8000));
  }

  return btoa(binary);
}

export async function downloadHandoffAsset(
  asset: unknown,
): Promise<AssetDownloadResponse> {
  if (!isHandoffAsset(asset)) {
    throw new Error("Invalid handoff asset download request.");
  }

  const response = await fetch(asset.source_url, {
    credentials: "omit",
  });

  if (!response.ok) {
    throw new Error(`Asset download failed with HTTP ${response.status}.`);
  }

  return {
    name: asset.name,
    mime_type: asset.mime_type || response.headers.get("Content-Type") || "",
    data_base64: arrayBufferToBase64(await response.arrayBuffer()),
  };
}

async function getMonitorState() {
  await recordCurrentHandoffExpiration();

  return {
    extension_id: browser.runtime.id,
    version: getManifestVersion(),
    current_handoff: await getCurrentHandoff(),
    events: await getExecutionEvents(),
    trusted_origins: await listTrustedOrigins(),
  };
}

async function getCurrentHandoffSnapshot(): Promise<CurrentHandoffSnapshot> {
  const [currentHandoff, events] = await Promise.all([
    getCurrentHandoff(),
    getExecutionEvents(),
  ]);

  return { currentHandoff, events };
}

export async function recordCurrentHandoffExpiration(
  snapshot?: CurrentHandoffSnapshot,
): Promise<boolean> {
  const { currentHandoff, events } =
    snapshot ?? (await getCurrentHandoffSnapshot());

  if (!currentHandoff || !isHandoffExpired(currentHandoff.handoff)) {
    return false;
  }

  let recordedExpiration = false;

  for (const platform of currentHandoff.handoff.platforms) {
    const latestEvent = getLatestPlatformEvent(events, platform.platform);

    if (
      latestEvent &&
      EXPIRATION_SUPPRESSION_STATUSES.has(latestEvent.status)
    ) {
      continue;
    }

    await recordAndCallbackEvent(platform, {
      platform: platform.platform,
      status: "expired",
      message: "Handoff expired before extension publishing completed.",
      metadata: {
        execution_id: currentHandoff.handoff.execution_id,
        expires_at: currentHandoff.handoff.expires_at,
      },
    });
    recordedExpiration = true;
  }

  return recordedExpiration;
}

function getLatestPlatformEvent(
  events: ExtensionExecutionEvent[],
  platform: PlatformKey,
): ExtensionExecutionEvent | null {
  return (
    events
      .slice()
      .reverse()
      .find((event) => event.platform === platform) ?? null
  );
}

export async function shouldRejectExpiredAdapterEvent(
  message: Extract<BackgroundMessage, { type: "adapter.event" }>,
  snapshot?: CurrentHandoffSnapshot,
): Promise<boolean> {
  const { currentHandoff, events } =
    snapshot ?? (await getCurrentHandoffSnapshot());

  if (!currentHandoff || !isHandoffExpired(currentHandoff.handoff)) {
    return false;
  }

  const latestEvent = getLatestPlatformEvent(events, message.event.platform);

  return (
    !latestEvent ||
    !POST_EXPIRATION_EVENT_ALLOWED_STATUSES.has(latestEvent.status)
  );
}

async function acceptBridgeHandoff(
  origin: string,
  rawHandoff: unknown,
): Promise<HandoffResponse> {
  if (!(await isTrustedOrigin(origin))) {
    return {
      accepted: false,
      reason: "origin_untrusted",
      message: "Approve this MPP origin before sending handoff data.",
      trust_url: getTrustOriginPageUrl(origin),
    };
  }

  return acceptValidatedHandoff(origin, rawHandoff);
}

async function acceptValidatedHandoff(
  sourceOrigin: string,
  rawHandoff: unknown,
): Promise<HandoffResponse> {
  const validation = validateHandoff(rawHandoff);

  if (!validation.ok) {
    return validation.rejection;
  }

  await storeAcceptedHandoff(validation.handoff, sourceOrigin);

  for (const platform of validation.handoff.platforms) {
    await recordAndCallbackEvent(platform, {
      platform: platform.platform,
      status: "accepted",
      message: "Extension accepted the publishing handoff.",
      metadata: {
        execution_id: validation.handoff.execution_id,
        source_origin: sourceOrigin,
      },
    });
  }

  void Promise.resolve(startPublishingTabs(validation.handoff)).catch(
    (error) => {
      console.warn("MPP extension publishing failed.", error);
    },
  );

  return {
    accepted: true,
    execution_id: validation.handoff.execution_id,
    platforms: validation.handoff.platforms,
  };
}

export async function acceptExtensionHandoff(
  rawHandoff: unknown,
): Promise<HandoffResponse> {
  return acceptValidatedHandoff("extension_workbench", rawHandoff);
}

async function handleAdapterEvent(
  message: Extract<BackgroundMessage, { type: "adapter.event" }>,
) {
  const snapshot = await getCurrentHandoffSnapshot();

  if (await shouldRejectExpiredAdapterEvent(message, snapshot)) {
    await recordCurrentHandoffExpiration(snapshot);
    return {
      ok: false,
      error: "Current handoff has expired.",
    };
  }

  await recordCurrentHandoffExpiration(snapshot);
  const { currentHandoff } = snapshot;

  if (
    !currentHandoff ||
    currentHandoff.handoff.execution_id !== message.execution_id
  ) {
    return {
      ok: false,
      error: "Adapter event does not match the current execution.",
    };
  }

  const platform = currentHandoff.handoff.platforms.find(
    (item) => item.platform === message.event.platform,
  );

  if (!platform) {
    return {
      ok: false,
      error: "Adapter event platform is not in the handoff.",
    };
  }

  await recordAndCallbackEvent(platform, message.event);
  return { ok: true };
}

async function handleBackgroundMessage(message: unknown): Promise<unknown> {
  if (!isBackgroundMessage(message)) {
    throw new Error("Unsupported background message.");
  }

  if (message.type === "bridge.detect") {
    return {
      installed: true,
      extension_id: browser.runtime.id,
      version: getManifestVersion(),
      schema_version: HANDOFF_SCHEMA_VERSION,
      trusted: await isTrustedOrigin(message.origin),
      trustable: isTrustableOrigin(message.origin),
      trust_url: getTrustOriginPageUrl(message.origin),
    };
  }

  if (message.type === "bridge.request_trust") {
    if (!isTrustableOrigin(message.origin)) {
      throw new Error("Origin is not eligible for trust.");
    }

    const trustUrl = getTrustOriginPageUrl(message.origin);
    await browser.tabs.create({ active: true, url: trustUrl });
    return { trust_url: trustUrl };
  }

  if (message.type === "bridge.publish_handoff") {
    return acceptBridgeHandoff(message.origin, message.handoff);
  }

  if (message.type === "extension.start_handoff") {
    return acceptExtensionHandoff(message.handoff);
  }

  if (message.type === "bridge.get_status") {
    if (!(await isTrustedOrigin(message.origin))) {
      return {
        trusted: false,
        trust_url: getTrustOriginPageUrl(message.origin),
      };
    }

    return getMonitorState();
  }

  if (message.type === "monitor.get") {
    return getMonitorState();
  }

  if (message.type === "monitor.clear") {
    await clearExecutionState();
    return getMonitorState();
  }

  if (message.type === "origin.trust") {
    const origin = await trustOrigin(message.origin);
    return { origin, trusted_origins: await listTrustedOrigins() };
  }

  if (message.type === "origin.list") {
    return { trusted_origins: await listTrustedOrigins() };
  }

  if (message.type === "origin.remove") {
    return {
      trusted_origins: await removeTrustedOrigin(message.origin),
    };
  }

  if (message.type === "adapter.event") {
    return handleAdapterEvent(message);
  }

  if (message.type === "asset.download") {
    return downloadHandoffAsset(message.asset);
  }

  throw new Error("Unknown background message type.");
}

async function openPublishMonitor(tabId?: number): Promise<void> {
  const sidePanel = browser.sidePanel;

  if (sidePanel?.open && tabId) {
    await sidePanel.open({ tabId });
    return;
  }

  await browser.tabs.create({
    active: true,
    url: browser.runtime.getURL("/publish.html"),
  });
}

export default defineBackground(() => {
  browser.action.onClicked.addListener((tab) => {
    openPublishMonitor(tab.id).catch((error) => {
      console.warn("Failed to open MPP publish monitor.", error);
    });
  });

  browser.runtime.onMessage.addListener((message, _sender, sendResponse) => {
    handleBackgroundMessage(message)
      .then(sendResponse)
      .catch((error) => {
        sendResponse({
          ok: false,
          error: error instanceof Error ? error.message : String(error),
        });
      });

    return true;
  });
});

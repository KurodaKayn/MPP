import { storage } from "#imports";
import { createExecutionEvent } from "../types/events";
import type {
  ExtensionExecutionEvent,
  ExtensionExecutionEventInput,
} from "../types/events";
import {
  HANDOFF_SCHEMA_VERSION,
  HANDOFF_TYPE,
  type AdaptedContent,
  type ExtensionPublishHandoff,
  type ExtensionPublishPlatformHandoff,
  type HandoffAsset,
  type HandoffCallback,
  type StoredHandoff,
} from "../types/handoff";
import {
  getCapabilityByAdapterKey,
  isCapabilityInjectUrl,
  isSupportedAdapterKey,
} from "../platforms/capabilities";
import type { HandoffRejectedResponse } from "../types/messages";

const currentHandoffItem = storage.defineItem<StoredHandoff | null>(
  "session:mpp.current_handoff",
  { fallback: null },
);
const executionEventsItem = storage.defineItem<ExtensionExecutionEvent[]>(
  "session:mpp.execution_events",
  { fallback: [] },
);

interface HandoffValidationSuccess {
  ok: true;
  handoff: ExtensionPublishHandoff;
}

interface HandoffValidationFailure {
  ok: false;
  rejection: HandoffRejectedResponse;
}

type HandoffValidationResult =
  | HandoffValidationSuccess
  | HandoffValidationFailure;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function readString(
  record: Record<string, unknown>,
  key: string,
): string | null {
  const value = record[key];
  return typeof value === "string" && value.length > 0 ? value : null;
}

function readBoolean(
  record: Record<string, unknown>,
  key: string,
): boolean | null {
  const value = record[key];
  return typeof value === "boolean" ? value : null;
}

function reject(
  reason: HandoffRejectedResponse["reason"],
  message: string,
): HandoffValidationFailure {
  return {
    ok: false,
    rejection: {
      accepted: false,
      reason,
      message,
    },
  };
}

function validateUrl(value: string): boolean {
  try {
    const url = new URL(value);
    return url.protocol === "https:" || url.protocol === "http:";
  } catch {
    return false;
  }
}

function validateAdaptedContent(
  value: unknown,
  adapterKey: ExtensionPublishPlatformHandoff["adapter_key"],
): AdaptedContent | null {
  if (!isRecord(value) || value.schema_version !== HANDOFF_SCHEMA_VERSION) {
    return null;
  }

  const capability = getCapabilityByAdapterKey(adapterKey);
  const format = readString(value, "format");

  if (!format || !capability.target_formats.includes(format as never)) {
    return null;
  }

  if (
    format === "markdown" &&
    typeof value.markdown === "string" &&
    value.markdown.length > 0
  ) {
    return {
      schema_version: HANDOFF_SCHEMA_VERSION,
      format,
      markdown: value.markdown,
    };
  }

  if (
    format === "html" &&
    typeof value.html === "string" &&
    value.html.length > 0
  ) {
    return {
      schema_version: HANDOFF_SCHEMA_VERSION,
      format,
      html: value.html,
    };
  }

  if (
    format === "text" &&
    typeof value.text === "string" &&
    value.text.length > 0
  ) {
    return {
      schema_version: HANDOFF_SCHEMA_VERSION,
      format,
      text: value.text,
    };
  }

  return null;
}

function validateAssets(value: unknown): HandoffAsset[] | null {
  if (!Array.isArray(value)) {
    return null;
  }

  const assets: HandoffAsset[] = [];

  for (const item of value) {
    if (!isRecord(item)) {
      return null;
    }

    const type = readString(item, "type");
    const sourceUrl = readString(item, "source_url");
    const name = readString(item, "name");
    const mimeType = readString(item, "mime_type");

    if (
      (type !== "image" && type !== "video") ||
      !sourceUrl ||
      !validateUrl(sourceUrl) ||
      !name ||
      !mimeType
    ) {
      return null;
    }

    assets.push({
      type,
      source_url: sourceUrl,
      name,
      mime_type: mimeType,
    });
  }

  return assets;
}

function validateCallback(value: unknown): HandoffCallback | undefined | null {
  if (value === undefined) {
    return undefined;
  }

  if (!isRecord(value)) {
    return null;
  }

  const url = readString(value, "url");
  const token = readString(value, "token");

  if (!url || !validateUrl(url) || !token) {
    return null;
  }

  return { url, token };
}

function validatePlatformHandoff(
  value: unknown,
): ExtensionPublishPlatformHandoff | null {
  if (!isRecord(value)) {
    return null;
  }

  const platform = readString(value, "platform");
  const adapterKey = readString(value, "adapter_key");
  const injectUrl = readString(value, "inject_url");
  const contentKind = readString(value, "content_kind");
  const autoPublish = readBoolean(value, "auto_publish");
  const requiresReview = readBoolean(value, "requires_review");

  if (!adapterKey || !isSupportedAdapterKey(adapterKey)) {
    return null;
  }

  const capability = getCapabilityByAdapterKey(adapterKey);
  const adaptedContent = validateAdaptedContent(
    value.adapted_content,
    adapterKey,
  );
  const assets = validateAssets(value.assets);
  const callback = validateCallback(value.callback);

  if (
    platform !== capability.platform ||
    !injectUrl ||
    !isCapabilityInjectUrl(adapterKey, injectUrl) ||
    !contentKind ||
    !capability.content_kinds.includes(contentKind as never) ||
    autoPublish === null ||
    requiresReview === null ||
    requiresReview !== capability.requires_review ||
    !adaptedContent ||
    !assets ||
    callback === null
  ) {
    return null;
  }

  if (autoPublish && !capability.auto_publish_allowed) {
    return null;
  }

  return {
    platform: capability.platform,
    adapter_key: adapterKey,
    inject_url: injectUrl,
    content_kind:
      contentKind as ExtensionPublishPlatformHandoff["content_kind"],
    auto_publish: autoPublish,
    requires_review: requiresReview,
    adapted_content: adaptedContent,
    assets,
    callback,
  };
}

export function validateHandoff(input: unknown): HandoffValidationResult {
  if (!isRecord(input)) {
    return reject("invalid_handoff", "Handoff must be an object.");
  }

  if (
    input.schema_version !== HANDOFF_SCHEMA_VERSION ||
    input.type !== HANDOFF_TYPE
  ) {
    return reject("invalid_schema", "Unsupported handoff schema.");
  }

  const executionId = readString(input, "execution_id");
  const expiresAt = readString(input, "expires_at");

  if (!executionId || !expiresAt) {
    return reject("invalid_handoff", "Handoff is missing execution metadata.");
  }

  const expiresAtTime = Date.parse(expiresAt);

  if (!Number.isFinite(expiresAtTime)) {
    return reject("invalid_handoff", "Handoff expiration is invalid.");
  }

  if (expiresAtTime <= Date.now()) {
    return reject("expired", "Handoff has expired.");
  }

  if (!isRecord(input.project)) {
    return reject("invalid_handoff", "Handoff is missing project metadata.");
  }

  const projectId = readString(input.project, "id");
  const projectTitle = readString(input.project, "title");

  if (!projectId || !projectTitle || !Array.isArray(input.platforms)) {
    return reject(
      "invalid_handoff",
      "Handoff project or platforms are invalid.",
    );
  }

  const platforms = input.platforms.map(validatePlatformHandoff);

  if (platforms.length === 0 || platforms.some((item) => item === null)) {
    return reject(
      "unsupported_adapter",
      "One or more platform adapters are unsupported.",
    );
  }

  return {
    ok: true,
    handoff: {
      schema_version: HANDOFF_SCHEMA_VERSION,
      type: HANDOFF_TYPE,
      execution_id: executionId,
      expires_at: expiresAt,
      project: {
        id: projectId,
        title: projectTitle,
      },
      platforms: platforms as ExtensionPublishPlatformHandoff[],
    },
  };
}

export async function storeAcceptedHandoff(
  handoff: ExtensionPublishHandoff,
  sourceOrigin: string,
): Promise<void> {
  await currentHandoffItem.setValue({
    handoff,
    accepted_at: new Date().toISOString(),
    source_origin: sourceOrigin,
  });
  await executionEventsItem.setValue([]);
}

export async function getCurrentHandoff(): Promise<StoredHandoff | null> {
  return currentHandoffItem.getValue();
}

export async function getExecutionEvents(): Promise<ExtensionExecutionEvent[]> {
  return executionEventsItem.getValue();
}

export async function appendExecutionEvent(
  input: ExtensionExecutionEventInput,
): Promise<ExtensionExecutionEvent> {
  const event = createExecutionEvent(input);
  const events = await executionEventsItem.getValue();
  await executionEventsItem.setValue([...events, event]);
  return event;
}

export async function clearExecutionState(): Promise<void> {
  await currentHandoffItem.setValue(null);
  await executionEventsItem.setValue([]);
}

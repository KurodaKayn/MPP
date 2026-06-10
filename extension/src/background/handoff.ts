import { storage } from "#imports";
import { createExecutionEvent } from "../types/events";
import type {
  ExtensionExecutionEvent,
  ExtensionExecutionEventInput,
  PublishExecutionStatus,
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
const executionQueueItem = storage.defineItem<StoredExecutionQueue | null>(
  "session:mpp.execution_queue",
  { fallback: null },
);

export type ExecutionQueueTaskStatus =
  | "queued"
  | Extract<
      PublishExecutionStatus,
      | "opening_tabs"
      | "injecting"
      | "user_review"
      | "submitted"
      | "succeeded"
      | "failed"
      | "cancelled"
      | "expired"
    >;

export interface StoredExecutionQueueTask {
  platform: ExtensionPublishPlatformHandoff["platform"];
  adapter_key: ExtensionPublishPlatformHandoff["adapter_key"];
  status: ExecutionQueueTaskStatus;
  tab_id?: number;
  error_message?: string;
  updated_at: string;
}

export interface StoredExecutionQueue {
  execution_id: string;
  project_id: string;
  active_platform: ExtensionPublishPlatformHandoff["platform"] | null;
  tasks: StoredExecutionQueueTask[];
  created_at: string;
  updated_at: string;
}

export type ExecutionQueueTaskUpdate = Partial<
  Pick<StoredExecutionQueueTask, "tab_id" | "error_message">
> & {
  status: ExecutionQueueTaskStatus;
};

const TERMINAL_QUEUE_TASK_STATUSES = new Set<ExecutionQueueTaskStatus>([
  "user_review",
  "submitted",
  "succeeded",
  "failed",
  "cancelled",
  "expired",
]);
const EXECUTION_QUEUE_TASK_STATUSES = new Set<ExecutionQueueTaskStatus>([
  "queued",
  "opening_tabs",
  "injecting",
  "user_review",
  "submitted",
  "succeeded",
  "failed",
  "cancelled",
  "expired",
]);

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

function isExpiredTimestamp(value: string): boolean {
  const expiresAtTime = Date.parse(value);

  return Number.isFinite(expiresAtTime) && expiresAtTime <= Date.now();
}

export function isHandoffExpired(handoff: ExtensionPublishHandoff): boolean {
  return isExpiredTimestamp(handoff.expires_at);
}

function getActiveQueuePlatform(
  tasks: StoredExecutionQueueTask[],
): StoredExecutionQueue["active_platform"] {
  return (
    tasks.find(
      (task) =>
        task.status !== "queued" &&
        !TERMINAL_QUEUE_TASK_STATUSES.has(task.status),
    )?.platform ?? null
  );
}

function createExecutionQueue(
  handoff: ExtensionPublishHandoff,
): StoredExecutionQueue {
  const now = new Date().toISOString();

  return {
    execution_id: handoff.execution_id,
    project_id: handoff.project.id,
    active_platform: null,
    tasks: handoff.platforms.map((platform) => ({
      platform: platform.platform,
      adapter_key: platform.adapter_key,
      status: "queued",
      updated_at: now,
    })),
    created_at: now,
    updated_at: now,
  };
}

export function isExecutionQueueTaskStatus(
  value: string,
): value is ExecutionQueueTaskStatus {
  return EXECUTION_QUEUE_TASK_STATUSES.has(value as ExecutionQueueTaskStatus);
}

export function isExecutionQueueActive(
  queue: StoredExecutionQueue | null,
): boolean {
  return (
    queue !== null &&
    queue.tasks.some((task) => !TERMINAL_QUEUE_TASK_STATUSES.has(task.status))
  );
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

  if (isExpiredTimestamp(expiresAt)) {
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
  await executionQueueItem.setValue(createExecutionQueue(handoff));
}

export async function getCurrentHandoff(): Promise<StoredHandoff | null> {
  return currentHandoffItem.getValue();
}

export async function getExecutionQueue(): Promise<StoredExecutionQueue | null> {
  return executionQueueItem.getValue();
}

export async function getExecutionEvents(): Promise<ExtensionExecutionEvent[]> {
  return executionEventsItem.getValue();
}

export async function appendExecutionEvent(
  input: ExtensionExecutionEventInput,
): Promise<ExtensionExecutionEvent> {
  const event = createExecutionEvent(input);

  return appendStoredExecutionEvent(event);
}

export async function appendStoredExecutionEvent(
  event: ExtensionExecutionEvent,
): Promise<ExtensionExecutionEvent> {
  const events = await executionEventsItem.getValue();
  await executionEventsItem.setValue([...events, event]);
  return event;
}

export async function updateExecutionQueueTask(
  executionId: string,
  platform: ExtensionPublishPlatformHandoff["platform"],
  update: ExecutionQueueTaskUpdate,
): Promise<StoredExecutionQueue | null> {
  const queue = await executionQueueItem.getValue();

  if (!queue || queue.execution_id !== executionId) {
    return queue;
  }

  const now = new Date().toISOString();
  const tasks = queue.tasks.map((task) =>
    task.platform === platform
      ? {
          ...task,
          ...update,
          updated_at: now,
        }
      : task,
  );
  const nextQueue: StoredExecutionQueue = {
    ...queue,
    active_platform: getActiveQueuePlatform(tasks),
    tasks,
    updated_at: now,
  };

  await executionQueueItem.setValue(nextQueue);
  return nextQueue;
}

export async function clearExecutionState(): Promise<void> {
  await currentHandoffItem.setValue(null);
  await executionEventsItem.setValue([]);
  await executionQueueItem.setValue(null);
}

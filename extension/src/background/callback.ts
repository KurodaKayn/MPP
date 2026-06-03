import type {
  ExtensionEventCallbackPayload,
  ExtensionExecutionEvent,
} from "../types/events";
import type { ExtensionPublishPlatformHandoff } from "../types/handoff";

const CALLBACK_TEXT_LIMIT = 500;

export function sanitizeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message.slice(0, CALLBACK_TEXT_LIMIT);
  }

  return String(error).slice(0, CALLBACK_TEXT_LIMIT);
}

function sanitizeCallbackText(value: string): string {
  return value.slice(0, CALLBACK_TEXT_LIMIT);
}

function createCallbackPayload(
  platform: ExtensionPublishPlatformHandoff,
  event: ExtensionExecutionEvent,
): ExtensionEventCallbackPayload {
  return {
    token: platform.callback?.token ?? "",
    event_id: event.event_id,
    platform: event.platform,
    status: event.status,
    message: sanitizeCallbackText(event.message),
    remote_id: sanitizeCallbackText(event.remote_id),
    publish_url: sanitizeCallbackText(event.publish_url),
    error_message: sanitizeCallbackText(event.error_message),
    metadata: event.metadata,
  };
}

export async function sendEventCallback(
  platform: ExtensionPublishPlatformHandoff,
  event: ExtensionExecutionEvent,
): Promise<void> {
  if (!platform.callback) {
    return;
  }

  const response = await fetch(platform.callback.url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(createCallbackPayload(platform, event)),
  });

  if (!response.ok) {
    throw new Error(`Callback rejected event with HTTP ${response.status}.`);
  }
}

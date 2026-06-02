import type { ExtensionExecutionEvent } from "../types/events";
import type { ExtensionPublishPlatformHandoff } from "../types/handoff";

export function sanitizeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message.slice(0, 500);
  }

  return String(error).slice(0, 500);
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
    body: JSON.stringify({
      token: platform.callback.token,
      event_id: event.event_id,
      platform: event.platform,
      status: event.status,
      message: event.message,
      remote_id: event.remote_id,
      publish_url: event.publish_url,
      error_message: event.error_message,
      metadata: event.metadata,
    }),
  });

  if (!response.ok) {
    throw new Error(`Callback rejected event with HTTP ${response.status}.`);
  }
}

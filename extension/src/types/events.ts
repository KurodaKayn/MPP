import type { PlatformKey } from "./platform";

export type PublishExecutionStatus =
  | "handoff_issued"
  | "accepted"
  | "opening_tabs"
  | "injecting"
  | "user_review"
  | "submitted"
  | "succeeded"
  | "failed"
  | "cancelled"
  | "expired";

export interface ExtensionExecutionEvent {
  event_id: string;
  platform: PlatformKey;
  status: PublishExecutionStatus;
  message: string;
  remote_id: string;
  publish_url: string;
  error_message: string;
  metadata: Record<string, unknown>;
  created_at: string;
}

export interface ExtensionExecutionEventInput {
  platform: PlatformKey;
  status: PublishExecutionStatus;
  message: string;
  remote_id?: string;
  publish_url?: string;
  error_message?: string;
  metadata?: Record<string, unknown>;
}

export interface ExtensionEventCallbackPayload {
  token: string;
  event_id: string;
  platform: PlatformKey;
  status: PublishExecutionStatus;
  message: string;
  remote_id: string;
  publish_url: string;
  error_message: string;
  metadata: Record<string, unknown>;
}

export function createExecutionEvent(
  input: ExtensionExecutionEventInput,
): ExtensionExecutionEvent {
  return {
    event_id: crypto.randomUUID(),
    platform: input.platform,
    status: input.status,
    message: input.message,
    remote_id: input.remote_id ?? "",
    publish_url: input.publish_url ?? "",
    error_message: input.error_message ?? "",
    metadata: input.metadata ?? {},
    created_at: new Date().toISOString(),
  };
}

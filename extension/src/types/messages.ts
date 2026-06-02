import type { ExtensionExecutionEventInput } from "./events";
import type {
  ExtensionPublishHandoff,
  ExtensionPublishPlatformHandoff,
} from "./handoff";
import type { AdapterKey } from "./platform";

export const PAGE_BRIDGE_REQUEST_CHANNEL = "mpp.extension.bridge.request";
export const PAGE_BRIDGE_RESPONSE_CHANNEL = "mpp.extension.bridge.response";

export const PAGE_BRIDGE_REQUEST_TYPES = [
  "detect",
  "request_trust",
  "publish_handoff",
  "get_status",
] as const;

export type PageBridgeRequestType = (typeof PAGE_BRIDGE_REQUEST_TYPES)[number];

export function isPageBridgeRequestType(
  value: string,
): value is PageBridgeRequestType {
  return PAGE_BRIDGE_REQUEST_TYPES.includes(value as PageBridgeRequestType);
}

export interface PageBridgeRequest {
  channel: typeof PAGE_BRIDGE_REQUEST_CHANNEL;
  request_id: string;
  type: PageBridgeRequestType;
  payload?: unknown;
}

export interface PageBridgeResponse {
  channel: typeof PAGE_BRIDGE_RESPONSE_CHANNEL;
  request_id: string;
  type: PageBridgeRequestType;
  ok: boolean;
  data?: unknown;
  error?: string;
}

export type BackgroundMessage =
  | {
      type: "bridge.detect";
      origin: string;
    }
  | {
      type: "bridge.request_trust";
      origin: string;
    }
  | {
      type: "bridge.publish_handoff";
      origin: string;
      handoff: unknown;
    }
  | {
      type: "bridge.get_status";
      origin: string;
    }
  | {
      type: "monitor.get";
    }
  | {
      type: "monitor.clear";
    }
  | {
      type: "origin.trust";
      origin: string;
    }
  | {
      type: "origin.list";
    }
  | {
      type: "origin.remove";
      origin: string;
    }
  | {
      type: "adapter.event";
      execution_id: string;
      event: ExtensionExecutionEventInput;
    };

export interface AdapterRunMessage {
  type: "adapter.run";
  execution_id: string;
  adapter_key: AdapterKey;
  project_title: string;
  platform: ExtensionPublishPlatformHandoff;
}

export interface HandoffAcceptedResponse {
  accepted: true;
  execution_id: string;
  platforms: ExtensionPublishHandoff["platforms"];
}

export interface HandoffRejectedResponse {
  accepted: false;
  reason:
    | "origin_untrusted"
    | "expired"
    | "invalid_schema"
    | "unsupported_adapter"
    | "invalid_handoff";
  message: string;
  trust_url?: string;
}

export type HandoffResponse = HandoffAcceptedResponse | HandoffRejectedResponse;

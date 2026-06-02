import {
  PAGE_BRIDGE_REQUEST_CHANNEL,
  PAGE_BRIDGE_RESPONSE_CHANNEL,
  type BackgroundMessage,
  type PageBridgeRequest,
  type PageBridgeResponse,
} from "../src/types/messages";

const DASHBOARD_MATCHES = [
  "https://mpp.example.com/*",
  "http://localhost/*",
  "http://127.0.0.1/*",
];

function isBridgeRequest(value: unknown): value is PageBridgeRequest {
  return (
    typeof value === "object" &&
    value !== null &&
    "channel" in value &&
    "request_id" in value &&
    "type" in value &&
    value.channel === PAGE_BRIDGE_REQUEST_CHANNEL &&
    typeof value.request_id === "string" &&
    typeof value.type === "string"
  );
}

function toBackgroundMessage(
  request: PageBridgeRequest,
  origin: string,
): BackgroundMessage | null {
  if (request.type === "detect") {
    return { type: "bridge.detect", origin };
  }

  if (request.type === "request_trust") {
    return { type: "bridge.request_trust", origin };
  }

  if (request.type === "publish_handoff") {
    return {
      type: "bridge.publish_handoff",
      origin,
      handoff: request.payload,
    };
  }

  if (request.type === "get_status") {
    return { type: "bridge.get_status", origin };
  }

  return null;
}

function postBridgeResponse(response: PageBridgeResponse): void {
  window.postMessage(response, window.location.origin);
}

export default defineContentScript({
  matches: DASHBOARD_MATCHES,
  runAt: "document_start",
  main() {
    window.addEventListener("message", (event) => {
      if (event.source !== window || !isBridgeRequest(event.data)) {
        return;
      }

      const backgroundMessage = toBackgroundMessage(
        event.data,
        window.location.origin,
      );

      if (!backgroundMessage) {
        postBridgeResponse({
          channel: PAGE_BRIDGE_RESPONSE_CHANNEL,
          request_id: event.data.request_id,
          type: event.data.type,
          ok: false,
          error: "Unsupported extension bridge request.",
        });
        return;
      }

      browser.runtime
        .sendMessage(backgroundMessage)
        .then((data) => {
          postBridgeResponse({
            channel: PAGE_BRIDGE_RESPONSE_CHANNEL,
            request_id: event.data.request_id,
            type: event.data.type,
            ok: true,
            data,
          });
        })
        .catch((error) => {
          postBridgeResponse({
            channel: PAGE_BRIDGE_RESPONSE_CHANNEL,
            request_id: event.data.request_id,
            type: event.data.type,
            ok: false,
            error: error instanceof Error ? error.message : String(error),
          });
        });
    });
  },
});

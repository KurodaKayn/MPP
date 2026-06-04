import {
  PAGE_BRIDGE_REQUEST_CHANNEL,
  PAGE_BRIDGE_RESPONSE_CHANNEL,
  type BackgroundMessage,
  type PageBridgeRequest,
  type PageBridgeResponse,
  isPageBridgeRequestType,
} from "../src/types/messages";
import { getWebAuthTokenFromStorage } from "../src/backend/auth";
import { startWebAuthTokenSync } from "../src/backend/web-auth-sync";

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
    value.request_id.length > 0 &&
    typeof value.type === "string" &&
    isPageBridgeRequestType(value.type)
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

function postBridgeResponse(
  response: PageBridgeResponse,
  targetOrigin: string,
): void {
  window.postMessage(response, targetOrigin);
}

async function persistPageAuthToken(): Promise<void> {
  const token = getWebAuthTokenFromStorage();

  if (!token) {
    return;
  }

  await browser.runtime.sendMessage({
    type: "extension.persist_auth_token",
    token,
  } satisfies BackgroundMessage);
}

async function forwardBackgroundMessage(
  message: BackgroundMessage,
): Promise<unknown> {
  await persistPageAuthToken().catch((error) => {
    console.warn("Failed to persist MPP page auth token.", error);
  });

  return browser.runtime.sendMessage(message);
}

export default defineContentScript({
  matches: DASHBOARD_MATCHES,
  runAt: "document_start",
  main() {
    startWebAuthTokenSync({
      onError: (error) => {
        console.warn("Failed to persist MPP page auth token.", error);
      },
      persistToken: (token) =>
        browser.runtime.sendMessage({
          type: "extension.persist_auth_token",
          token,
        } satisfies BackgroundMessage),
      readToken: getWebAuthTokenFromStorage,
    });

    window.addEventListener("message", (event) => {
      if (
        event.source !== window ||
        event.origin !== window.location.origin ||
        !isBridgeRequest(event.data)
      ) {
        return;
      }

      const backgroundMessage = toBackgroundMessage(event.data, event.origin);

      if (!backgroundMessage) {
        postBridgeResponse(
          {
            channel: PAGE_BRIDGE_RESPONSE_CHANNEL,
            request_id: event.data.request_id,
            type: event.data.type,
            ok: false,
            error: "Unsupported extension bridge request.",
          },
          event.origin,
        );
        return;
      }

      forwardBackgroundMessage(backgroundMessage)
        .then((data) => {
          postBridgeResponse(
            {
              channel: PAGE_BRIDGE_RESPONSE_CHANNEL,
              request_id: event.data.request_id,
              type: event.data.type,
              ok: true,
              data,
            },
            event.origin,
          );
        })
        .catch((error) => {
          postBridgeResponse(
            {
              channel: PAGE_BRIDGE_RESPONSE_CHANNEL,
              request_id: event.data.request_id,
              type: event.data.type,
              ok: false,
              error: error instanceof Error ? error.message : String(error),
            },
            event.origin,
          );
        });
    });
  },
});

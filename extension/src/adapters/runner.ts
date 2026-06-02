import type { AdapterResult } from "./shared";
import type { AdapterRunMessage, BackgroundMessage } from "../types/messages";
import type { AdapterKey } from "../types/platform";

type AdapterRunner = (
  platform: AdapterRunMessage["platform"],
  projectTitle: string,
) => Promise<AdapterResult>;

function isAdapterRunMessage(
  message: unknown,
  adapterKey: AdapterKey,
): message is AdapterRunMessage {
  return (
    typeof message === "object" &&
    message !== null &&
    "type" in message &&
    "adapter_key" in message &&
    message.type === "adapter.run" &&
    message.adapter_key === adapterKey
  );
}

async function reportAdapterResult(
  message: AdapterRunMessage,
  result: AdapterResult,
): Promise<void> {
  const event: BackgroundMessage = {
    type: "adapter.event",
    execution_id: message.execution_id,
    event: {
      platform: message.platform.platform,
      status: result.status,
      message: result.message,
      error_message: result.error_message,
      metadata: {
        ...result.metadata,
        adapter_key: message.adapter_key,
        url: location.href,
      },
    },
  };

  await browser.runtime.sendMessage(event);
}

export function registerAdapterRunner(
  adapterKey: AdapterKey,
  runner: AdapterRunner,
): void {
  browser.runtime.onMessage.addListener((message, _sender, sendResponse) => {
    if (!isAdapterRunMessage(message, adapterKey)) {
      return false;
    }

    runner(message.platform, message.project_title)
      .then(async (result) => {
        await reportAdapterResult(message, result);
        sendResponse({
          ok: result.status !== "failed",
          result,
        });
      })
      .catch(async (error) => {
        const result: AdapterResult = {
          status: "failed",
          message: "Adapter execution failed.",
          error_message: error instanceof Error ? error.message : String(error),
        };
        await reportAdapterResult(message, result);
        sendResponse({ ok: false, result });
      });

    return true;
  });
}

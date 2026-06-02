import {
  ADAPTER_SCRIPT_FILES,
  isCapabilityInjectUrl,
} from "../platforms/capabilities";
import type { ExtensionExecutionEventInput } from "../types/events";
import type {
  ExtensionPublishHandoff,
  ExtensionPublishPlatformHandoff,
} from "../types/handoff";
import type { AdapterRunMessage } from "../types/messages";
import { appendExecutionEvent } from "./handoff";
import { sanitizeError, sendEventCallback } from "./callback";

const TAB_LOAD_TIMEOUT_MS = 45_000;

function waitForTabComplete(tabId: number): Promise<void> {
  return new Promise((resolve, reject) => {
    let timeoutId: ReturnType<typeof globalThis.setTimeout> | undefined;

    const cleanup = () => {
      if (timeoutId !== undefined) {
        globalThis.clearTimeout(timeoutId);
      }
      browser.tabs.onUpdated.removeListener(listener);
    };

    const listener = (
      updatedTabId: number,
      changeInfo: { status?: string },
    ) => {
      if (updatedTabId === tabId && changeInfo.status === "complete") {
        cleanup();
        resolve();
      }
    };

    timeoutId = globalThis.setTimeout(() => {
      cleanup();
      reject(new Error("Timed out waiting for platform tab to load."));
    }, TAB_LOAD_TIMEOUT_MS);

    browser.tabs.onUpdated.addListener(listener);
  });
}

export async function recordAndCallbackEvent(
  platform: ExtensionPublishPlatformHandoff,
  input: ExtensionExecutionEventInput,
): Promise<void> {
  const event = await appendExecutionEvent(input);

  try {
    await sendEventCallback(platform, event);
  } catch (error) {
    console.warn("MPP extension callback failed.", sanitizeError(error));
  }
}

async function assertInjectableTabUrl(
  tabId: number,
  platform: ExtensionPublishPlatformHandoff,
): Promise<void> {
  const tab = await browser.tabs.get(tabId);

  if (!tab.url || !isCapabilityInjectUrl(platform.adapter_key, tab.url)) {
    throw new Error("Platform tab did not load the expected publishing page.");
  }
}

async function injectPlatformAdapter(
  executionId: string,
  projectTitle: string,
  tabId: number,
  platform: ExtensionPublishPlatformHandoff,
): Promise<void> {
  const scriptFile = ADAPTER_SCRIPT_FILES[platform.adapter_key];

  await browser.scripting.executeScript({
    target: { tabId },
    files: [scriptFile],
  });

  const message: AdapterRunMessage = {
    type: "adapter.run",
    execution_id: executionId,
    adapter_key: platform.adapter_key,
    project_title: projectTitle,
    platform,
  };

  await browser.tabs.sendMessage(tabId, message);
}

async function openAndInjectPlatform(
  handoff: ExtensionPublishHandoff,
  platform: ExtensionPublishPlatformHandoff,
): Promise<void> {
  await recordAndCallbackEvent(platform, {
    platform: platform.platform,
    status: "opening_tabs",
    message: "Opening platform publishing page.",
    metadata: {
      url: platform.inject_url,
    },
  });

  const tab = await browser.tabs.create({
    active: true,
    url: platform.inject_url,
  });

  if (!tab.id) {
    throw new Error("Platform tab did not return an id.");
  }

  await waitForTabComplete(tab.id);
  await assertInjectableTabUrl(tab.id, platform);

  await recordAndCallbackEvent(platform, {
    platform: platform.platform,
    status: "injecting",
    message: "Injecting platform adapter.",
    metadata: {
      tab_id: tab.id,
      url: platform.inject_url,
      adapter_key: platform.adapter_key,
    },
  });

  await injectPlatformAdapter(
    handoff.execution_id,
    handoff.project.title,
    tab.id,
    platform,
  );
}

export async function startPublishingTabs(
  handoff: ExtensionPublishHandoff,
): Promise<void> {
  for (const platform of handoff.platforms) {
    try {
      await openAndInjectPlatform(handoff, platform);
    } catch (error) {
      await recordAndCallbackEvent(platform, {
        platform: platform.platform,
        status: "failed",
        message: "Failed to run platform adapter.",
        error_message: sanitizeError(error),
        metadata: {
          url: platform.inject_url,
          adapter_key: platform.adapter_key,
        },
      });
    }
  }
}

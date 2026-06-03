import type { ExtensionPublishPlatformHandoff } from "../types/handoff";
import type { PublishExecutionStatus } from "../types/events";

export interface AdapterResult {
  status: Extract<
    PublishExecutionStatus,
    "user_review" | "succeeded" | "failed"
  >;
  message: string;
  error_message?: string;
  metadata?: Record<string, unknown>;
}

type TextTarget = HTMLInputElement | HTMLTextAreaElement | HTMLElement;

export function getDraftText(
  platform: ExtensionPublishPlatformHandoff,
): string {
  const content = platform.adapted_content;

  if (content.format === "markdown") {
    return content.markdown ?? "";
  }

  if (content.format === "html") {
    return content.html ?? "";
  }

  return content.text ?? "";
}

export function isOnExpectedHost(expectedHosts: string[]): boolean {
  return expectedHosts.some((host) => location.hostname.endsWith(host));
}

export function findFirstElement<T extends Element>(
  selectors: string[],
): T | null {
  for (const selector of selectors) {
    const element = document.querySelector<T>(selector);

    if (element) {
      return element;
    }
  }

  return null;
}

export function fillTextTarget(target: TextTarget, value: string): void {
  target.focus();

  if (
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement
  ) {
    target.value = value;
    target.dispatchEvent(new Event("input", { bubbles: true }));
    target.dispatchEvent(new Event("change", { bubbles: true }));
    return;
  }

  if (
    target.isContentEditable ||
    target.getAttribute("contenteditable") === "true"
  ) {
    target.textContent = value;
    target.dispatchEvent(new InputEvent("input", { bubbles: true }));
  }
}

export function failed(message: string, error?: unknown): AdapterResult {
  return {
    status: "failed",
    message,
    error_message: error instanceof Error ? error.message : String(error ?? ""),
  };
}

export function userReview(
  message: string,
  metadata?: Record<string, unknown>,
): AdapterResult {
  return {
    status: "user_review",
    message,
    metadata,
  };
}

import type { ExtensionPublishPlatformHandoff } from "../types/handoff";
import type { AdapterResult } from "./shared";
import {
  failed,
  fillTextTarget,
  findFirstElement,
  getDraftText,
  userReview,
} from "./shared";

const X_HOSTS = ["x.com", "twitter.com"];
const COMPOSER_WAIT_TIMEOUT_MS = 10_000;
const COMPOSER_WAIT_INTERVAL_MS = 250;
const X_COMPOSER_SELECTORS = [
  '[data-testid="tweetTextarea_0"][contenteditable="true"]',
  '[data-testid^="tweetTextarea_"][contenteditable="true"]',
  '[role="textbox"][contenteditable="true"][aria-label*="Post"]',
  '[role="textbox"][contenteditable="true"][aria-label*="Tweet"]',
  '.public-DraftEditor-content[contenteditable="true"]',
  '[role="textbox"][contenteditable="true"]',
  '[contenteditable="true"][data-testid*="tweetTextarea"]',
];
const X_SIGN_OUT_SELECTORS = [
  'input[type="password"]',
  'a[href*="/login"]',
  'a[href*="/i/flow/login"]',
  'a[href*="/i/flow/signup"]',
  'button[data-testid="loginButton"]',
];

function wait(milliseconds: number): Promise<void> {
  return new Promise((resolve) => {
    globalThis.setTimeout(resolve, milliseconds);
  });
}

function isOnXHost(): boolean {
  return X_HOSTS.some(
    (host) =>
      location.hostname === host || location.hostname.endsWith(`.${host}`),
  );
}

function hasAnyElement(selectors: string[]): boolean {
  return selectors.some(
    (selector) => document.querySelector(selector) !== null,
  );
}

function hasVisibleText(values: string[]): boolean {
  const bodyText = document.body.textContent?.toLowerCase() ?? "";

  return values.some((value) => bodyText.includes(value.toLowerCase()));
}

function isXSignInUiVisible(): boolean {
  return (
    hasAnyElement(X_SIGN_OUT_SELECTORS) ||
    hasVisibleText(["Sign in to X", "Log in to X", "Log in", "Sign in"])
  );
}

async function waitForXComposer(): Promise<HTMLElement | null> {
  const startedAt = Date.now();

  while (Date.now() - startedAt < COMPOSER_WAIT_TIMEOUT_MS) {
    const composer = findFirstElement<HTMLElement>(X_COMPOSER_SELECTORS);

    if (composer) {
      return composer;
    }

    await wait(COMPOSER_WAIT_INTERVAL_MS);
  }

  return findFirstElement<HTMLElement>(X_COMPOSER_SELECTORS);
}

function fillXComposer(composer: HTMLElement, text: string): void {
  fillTextTarget(composer, text);
}

export async function runXPostAdapter(
  platform: ExtensionPublishPlatformHandoff,
  _projectTitle: string,
): Promise<AdapterResult> {
  if (!isOnXHost()) {
    return failed("X adapter can only run on X compose pages.");
  }

  if (isXSignInUiVisible()) {
    return failed(
      "Please sign in to X before publishing.",
      "X sign-in UI detected.",
    );
  }

  const composer = await waitForXComposer();

  if (!composer) {
    return failed(
      "Could not find the X post composer.",
      "X composer textbox was not found.",
    );
  }

  const draftText = getDraftText(platform);

  fillXComposer(composer, draftText);

  return userReview("X draft prepared. Review and post manually.", {
    auto_publish: false,
    content_kind: platform.content_kind,
    text_length: draftText.length,
  });
}

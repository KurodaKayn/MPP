import { beforeEach, describe, expect, it, vi } from "vitest";
import { runXPostAdapter } from "./x-post";
import type { ExtensionPublishPlatformHandoff } from "../types/handoff";

function createXPlatform(
  text = "MPP X draft text",
): ExtensionPublishPlatformHandoff {
  return {
    platform: "x",
    adapter_key: "POST_X",
    inject_url: "https://x.com/compose/post",
    content_kind: "dynamic_post",
    auto_publish: false,
    requires_review: true,
    adapted_content: {
      schema_version: 1,
      format: "text",
      text,
    },
    assets: [],
  };
}

function renderXComposer(): HTMLElement {
  document.body.innerHTML = `
    <div data-testid="tweetTextarea_0" role="textbox" contenteditable="true" aria-label="Post text"></div>
    <button data-testid="tweetButton" type="button">Post</button>
  `;

  const composer = document.querySelector<HTMLElement>(
    '[data-testid="tweetTextarea_0"]',
  );

  if (!composer) {
    throw new Error("Test composer was not rendered.");
  }

  return composer;
}

function renderXDraftComposer(): HTMLElement {
  document.body.innerHTML = `
    <div
      aria-label="Post text"
      aria-multiline="true"
      class="notranslate public-DraftEditor-content"
      contenteditable="true"
      data-testid="tweetTextarea_0"
      role="textbox"
      spellcheck="true"
      tabindex="0"
    >
      <div data-contents="true">
        <div data-block="true" data-editor="editor-id" data-offset-key="block-0-0">
          <div data-offset-key="block-0-0" class="public-DraftStyleDefault-block public-DraftStyleDefault-ltr">
            <span data-offset-key="block-0-0"><span data-text="true"></span></span>
          </div>
        </div>
      </div>
    </div>
    <button data-testid="tweetButton" type="button" disabled>Post</button>
  `;

  const composer = document.querySelector<HTMLElement>(
    '[data-testid="tweetTextarea_0"]',
  );

  if (!composer) {
    throw new Error("Test Draft.js composer was not rendered.");
  }

  return composer;
}

function renderXEmptyDraftComposer(): HTMLElement {
  document.body.innerHTML = `
    <div
      aria-label="Post text"
      aria-multiline="true"
      class="notranslate public-DraftEditor-content"
      contenteditable="true"
      data-testid="tweetTextarea_0"
      role="textbox"
      spellcheck="true"
      tabindex="0"
    >
      <div data-contents="true">
        <div data-block="true" data-editor="editor-id" data-offset-key="block-0-0">
          <div data-offset-key="block-0-0" class="public-DraftStyleDefault-block public-DraftStyleDefault-ltr">
            <span data-offset-key="block-0-0"><br data-text="true"></span>
          </div>
        </div>
      </div>
    </div>
    <button data-testid="tweetButton" type="button" disabled>Post</button>
  `;

  const composer = document.querySelector<HTMLElement>(
    '[data-testid="tweetTextarea_0"]',
  );

  if (!composer) {
    throw new Error("Test empty Draft.js composer was not rendered.");
  }

  return composer;
}

describe("runXPostAdapter", () => {
  beforeEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    Reflect.deleteProperty(document, "execCommand");
    document.body.innerHTML = "";
    vi.stubGlobal("location", {
      hostname: "x.com",
      href: "https://x.com/compose/post",
      pathname: "/compose/post",
    });
  });

  it("fills the X composer and leaves final posting to the user", async () => {
    const composer = renderXComposer();
    const postButton = document.querySelector<HTMLButtonElement>(
      '[data-testid="tweetButton"]',
    );
    const postClick = vi.fn();
    postButton?.addEventListener("click", postClick);

    const result = await runXPostAdapter(
      createXPlatform("Draft for X"),
      "Project title",
    );

    expect(result.status).toBe("user_review");
    expect(result.message).toBe("X draft prepared. Review and post manually.");
    expect(result.metadata).toMatchObject({
      auto_publish: false,
      content_kind: "dynamic_post",
      text_length: "Draft for X".length,
    });
    expect(composer.textContent).toBe("Draft for X");
    expect(postClick).not.toHaveBeenCalled();
  });

  it("inserts text through the native editing path for Draft.js composers", async () => {
    const composer = renderXDraftComposer();
    const draftTextLeaf =
      composer.querySelector<HTMLElement>('[data-text="true"]');
    const selectedNodes: Node[] = [];
    const inputEvents: InputEvent[] = [];
    const originalSelectNodeContents = Range.prototype.selectNodeContents;
    const execCommand = vi.fn(
      (_command: string, _showUi: boolean, value?: string) => {
        draftTextLeaf!.textContent = value ?? "";
        composer.dispatchEvent(
          new InputEvent("input", {
            bubbles: true,
            data: value,
            inputType: "insertText",
          }),
        );
        return true;
      },
    );
    const selectNodeContents = vi.spyOn(Range.prototype, "selectNodeContents");

    selectNodeContents.mockImplementation(function (this: Range, node: Node) {
      selectedNodes.push(node);
      return originalSelectNodeContents.call(this, node);
    });

    Object.defineProperty(document, "execCommand", {
      configurable: true,
      value: execCommand,
    });
    composer.addEventListener("input", (event) => {
      inputEvents.push(event as InputEvent);
    });

    const result = await runXPostAdapter(
      createXPlatform("Draft for X"),
      "Project title",
    );

    expect(result.status).toBe("user_review");
    expect(execCommand).toHaveBeenCalledWith(
      "insertText",
      false,
      "Draft for X",
    );
    expect(selectedNodes.at(-1)).toBe(draftTextLeaf);
    expect(inputEvents.at(-1)).toMatchObject({
      inputType: "insertText",
      data: "Draft for X",
    });
    expect(composer.textContent).toContain("Draft for X");
  });

  it("selects the empty Draft.js text wrapper instead of its placeholder break", async () => {
    const composer = renderXEmptyDraftComposer();
    const placeholderBreak = composer.querySelector<HTMLElement>(
      'br[data-text="true"]',
    );
    const offsetTextWrapper = placeholderBreak?.closest<HTMLElement>(
      "span[data-offset-key]",
    );
    const setStartCalls: Array<{ node: Node; offset: number }> = [];
    const originalSetStart = Range.prototype.setStart;
    const execCommand = vi.fn(() => true);
    const setStart = vi.spyOn(Range.prototype, "setStart");

    setStart.mockImplementation(function (
      this: Range,
      node: Node,
      offset: number,
    ) {
      setStartCalls.push({ node, offset });
      return originalSetStart.call(this, node, offset);
    });
    Object.defineProperty(document, "execCommand", {
      configurable: true,
      value: execCommand,
    });

    const result = await runXPostAdapter(
      createXPlatform("Draft for X"),
      "Project title",
    );

    expect(result.status).toBe("user_review");
    expect(setStartCalls.at(-1)).toEqual({
      node: offsetTextWrapper,
      offset: 0,
    });
    expect(setStartCalls.at(-1)?.node).not.toBe(placeholderBreak);
  });

  it("uses markdown content when text content is unavailable", async () => {
    const composer = renderXComposer();
    const platform = createXPlatform("");
    platform.adapted_content = {
      schema_version: 1,
      format: "markdown",
      markdown: "**Markdown draft**",
    };

    const result = await runXPostAdapter(platform, "Project title");

    expect(result.status).toBe("user_review");
    expect(composer.textContent).toBe("**Markdown draft**");
  });

  it("fails clearly when the user is signed out of X", async () => {
    document.body.innerHTML = `
      <form>
        <input type="password" aria-label="Password">
        <button type="submit">Log in</button>
      </form>
    `;

    const result = await runXPostAdapter(createXPlatform(), "Project title");

    expect(result).toMatchObject({
      status: "failed",
      message: "Please sign in to X before publishing.",
      error_message: "X sign-in UI detected.",
    });
  });

  it("fails clearly when the X composer is unavailable", async () => {
    vi.useFakeTimers();
    document.body.innerHTML = `<main>Home timeline</main>`;

    const resultPromise = runXPostAdapter(createXPlatform(), "Project title");

    await vi.advanceTimersByTimeAsync(10_250);

    const result = await resultPromise;

    expect(result).toMatchObject({
      status: "failed",
      message: "Could not find the X post composer.",
    });
  });

  it("fails when it is injected outside X", async () => {
    vi.stubGlobal("location", {
      hostname: "example.com",
      href: "https://example.com/compose/post",
      pathname: "/compose/post",
    });
    renderXComposer();

    const result = await runXPostAdapter(createXPlatform(), "Project title");

    expect(result).toMatchObject({
      status: "failed",
      message: "X adapter can only run on X compose pages.",
    });
  });
});

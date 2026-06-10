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

describe("runXPostAdapter", () => {
  beforeEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
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

import { beforeEach, describe, expect, it, vi } from "vitest";
import { runBilibiliDynamicAdapter } from "./bilibili-dynamic";
import { runXiaohongshuNoteAdapter } from "./xiaohongshu-note";
import { runZhihuArticleAdapter } from "./zhihu-article";
import type { ExtensionPublishPlatformHandoff } from "../types/handoff";
import zhCn from "../i18n/zh-CN.json";

const { xiaohongshu, zhihu } = zhCn.adapters;

function createPlatform(
  overrides: Partial<ExtensionPublishPlatformHandoff> = {},
): ExtensionPublishPlatformHandoff {
  return {
    platform: "zhihu",
    adapter_key: "ARTICLE_ZHIHU",
    inject_url: "https://zhuanlan.zhihu.com/write",
    content_kind: "article",
    auto_publish: false,
    requires_review: true,
    adapted_content: {
      schema_version: 1,
      format: "markdown",
      markdown: "Draft body",
    },
    assets: [],
    ...overrides,
  };
}

function stubLocation(hostname: string, href = `https://${hostname}/`) {
  vi.stubGlobal("location", {
    hostname,
    href,
    pathname: new URL(href).pathname,
  });
}

describe("runZhihuArticleAdapter", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
    stubLocation("zhuanlan.zhihu.com", "https://zhuanlan.zhihu.com/write");
  });

  it("fills the Zhihu title and article body", async () => {
    document.body.innerHTML = `
      <a href="/people/creator">Creator</a>
      <textarea placeholder="${zhihu.titlePlaceholder}"></textarea>
      <div contenteditable="true" data-placeholder="${zhihu.bodyPlaceholder}"></div>
    `;
    const body = document.querySelector<HTMLElement>(
      '[contenteditable="true"]',
    );
    const title = document.querySelector<HTMLTextAreaElement>("textarea");

    const result = await runZhihuArticleAdapter(
      createPlatform(),
      "Project title",
    );

    expect(result).toMatchObject({
      status: "user_review",
      message: "Draft filled. Waiting for user review.",
      metadata: {
        account_status: "signed_in",
        assets: 0,
        auto_publish: false,
      },
    });
    expect(title?.value).toBe("Project title");
    expect(body?.textContent).toBe("Draft body");
  });

  it("fails clearly when the user is signed out of Zhihu", async () => {
    document.body.innerHTML = `
      <form>
        <button type="submit">Sign in</button>
      </form>
    `;

    const result = await runZhihuArticleAdapter(
      createPlatform(),
      "Project title",
    );

    expect(result).toMatchObject({
      status: "failed",
      message: "Please sign in to Zhihu before publishing.",
      error_message: "Zhihu sign-in UI detected.",
    });
  });

  it("fails clearly when the Zhihu editor is missing", async () => {
    document.body.innerHTML = `<a href="/people/creator">Creator</a>`;

    const result = await runZhihuArticleAdapter(
      createPlatform(),
      "Project title",
    );

    expect(result).toMatchObject({
      status: "failed",
      message: "Could not find the Zhihu article editor.",
    });
  });

  it("fails when injected outside Zhihu", async () => {
    stubLocation("example.com");
    document.body.innerHTML = `
      <a href="/people/creator">Creator</a>
      <textarea></textarea>
    `;

    const result = await runZhihuArticleAdapter(
      createPlatform(),
      "Project title",
    );

    expect(result).toMatchObject({
      status: "failed",
      message: "Zhihu adapter can only run on Zhihu editor pages.",
    });
  });
});

describe("runBilibiliDynamicAdapter", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
    stubLocation("t.bilibili.com");
  });

  it("fills the Bilibili dynamic editor", async () => {
    document.body.innerHTML = `<div contenteditable="true"></div>`;
    const body = document.querySelector<HTMLElement>(
      '[contenteditable="true"]',
    );

    const result = await runBilibiliDynamicAdapter(
      createPlatform({
        platform: "bilibili",
        adapter_key: "DYNAMIC_BILIBILI",
        inject_url: "https://t.bilibili.com",
        content_kind: "dynamic_post",
        adapted_content: {
          schema_version: 1,
          format: "text",
          text: "Bilibili draft",
        },
      }),
      "Project title",
    );

    expect(result).toMatchObject({
      status: "user_review",
      message: "Dynamic text prepared. Waiting for user review.",
      metadata: {
        account_status: "signed_in",
        assets: 0,
        auto_publish: false,
      },
    });
    expect(body?.textContent).toBe("Bilibili draft");
  });

  it("fails clearly when the user is signed out of Bilibili", async () => {
    document.body.innerHTML = `
      <form>
        <input type="password" aria-label="Password">
        <button type="submit">Sign in</button>
      </form>
    `;

    const result = await runBilibiliDynamicAdapter(
      createPlatform({ platform: "bilibili", adapter_key: "DYNAMIC_BILIBILI" }),
      "Project title",
    );

    expect(result).toMatchObject({
      status: "failed",
      message: "Please sign in to Bilibili before publishing.",
      error_message: "Sign-in UI detected.",
    });
  });

  it("fails clearly when the Bilibili editor is missing", async () => {
    document.body.innerHTML = `<main>Creator home</main>`;

    const result = await runBilibiliDynamicAdapter(
      createPlatform({ platform: "bilibili", adapter_key: "DYNAMIC_BILIBILI" }),
      "Project title",
    );

    expect(result).toMatchObject({
      status: "failed",
      message: "Could not find the Bilibili dynamic editor.",
    });
  });

  it("fails when injected outside Bilibili", async () => {
    stubLocation("example.com");
    document.body.innerHTML = `<div contenteditable="true"></div>`;

    const result = await runBilibiliDynamicAdapter(
      createPlatform({ platform: "bilibili", adapter_key: "DYNAMIC_BILIBILI" }),
      "Project title",
    );

    expect(result).toMatchObject({
      status: "failed",
      message: "Bilibili adapter can only run on Bilibili pages.",
    });
  });
});

describe("runXiaohongshuNoteAdapter", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
    stubLocation("creator.xiaohongshu.com");
  });

  it("fills the Xiaohongshu note editor", async () => {
    document.body.innerHTML = `<textarea placeholder="${xiaohongshu.descriptionPlaceholder}"></textarea>`;
    const body = document.querySelector<HTMLTextAreaElement>("textarea");

    const result = await runXiaohongshuNoteAdapter(
      createPlatform({
        platform: "xiaohongshu",
        adapter_key: "NOTE_XIAOHONGSHU",
        inject_url: "https://creator.xiaohongshu.com/publish/publish",
        content_kind: "image_note",
        adapted_content: {
          schema_version: 1,
          format: "text",
          text: "Xiaohongshu draft",
        },
        assets: [
          {
            type: "image",
            source_url: "https://assets.example.com/note.png",
            name: "note.png",
            mime_type: "image/png",
          },
        ],
      }),
      "Project title",
    );

    expect(result).toMatchObject({
      status: "user_review",
      message:
        "Note text prepared. Upload assets and review before publishing.",
      metadata: {
        account_status: "signed_in",
        assets: 1,
        auto_publish: false,
      },
    });
    expect(body?.value).toBe("Xiaohongshu draft");
  });

  it("fails clearly when the user is signed out of Xiaohongshu", async () => {
    document.body.innerHTML = `
      <form>
        <input type="password" aria-label="Password">
        <button type="submit">Sign in</button>
      </form>
    `;

    const result = await runXiaohongshuNoteAdapter(
      createPlatform({
        platform: "xiaohongshu",
        adapter_key: "NOTE_XIAOHONGSHU",
      }),
      "Project title",
    );

    expect(result).toMatchObject({
      status: "failed",
      message: "Please sign in to Xiaohongshu before publishing.",
      error_message: "Sign-in UI detected.",
    });
  });

  it("fails clearly when the Xiaohongshu editor is missing", async () => {
    document.body.innerHTML = `<main>Creator home</main>`;

    const result = await runXiaohongshuNoteAdapter(
      createPlatform({
        platform: "xiaohongshu",
        adapter_key: "NOTE_XIAOHONGSHU",
      }),
      "Project title",
    );

    expect(result).toMatchObject({
      status: "failed",
      message: "Could not find the Xiaohongshu note editor.",
    });
  });

  it("fails when injected outside Xiaohongshu", async () => {
    stubLocation("example.com");
    document.body.innerHTML = `<textarea></textarea>`;

    const result = await runXiaohongshuNoteAdapter(
      createPlatform({
        platform: "xiaohongshu",
        adapter_key: "NOTE_XIAOHONGSHU",
      }),
      "Project title",
    );

    expect(result).toMatchObject({
      status: "failed",
      message: "Xiaohongshu adapter can only run on Xiaohongshu creator pages.",
    });
  });
});

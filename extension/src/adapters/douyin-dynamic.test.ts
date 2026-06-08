import { beforeEach, describe, expect, it, vi } from "vitest";
import { assetToFile, runDouyinDynamicAdapter } from "./douyin-dynamic";
import type {
  ExtensionPublishPlatformHandoff,
  HandoffAsset,
} from "../types/handoff";
import zhCn from "../i18n/zh-CN.json";

const asset: HandoffAsset = {
  type: "image",
  source_url: "https://assets.example.com/douyin-cover.png",
  name: "douyin-cover.png",
  mime_type: "image/png",
};
const { douyin } = zhCn.adapters;
const {
  defaultArticleText,
  articleText,
  articleSummary,
  articleTitle,
  shortBody,
  shortTitle,
  fullTitlePlaceholder,
  fullSummaryPlaceholder,
} = zhCn.tests.douyinDynamic;
const articleButtonText = douyin.articleButtonText;
const articleImageUploadText = douyin.articleImageUploadText;
const articleAiGenerateText = douyin.articleAiGenerateText;
const titlePlaceholder = douyin.articleTitlePlaceholder;
const summaryPlaceholder = douyin.articleSummaryPlaceholder;
const bodyPlaceholder = douyin.bodyPlaceholder;

describe("assetToFile", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it("requests asset downloads through the extension background", async () => {
    const sendMessage = vi.fn(() =>
      Promise.resolve({
        name: "douyin-cover.png",
        mime_type: "image/png",
        data_base64: "SGVsbG8gRG91eWlu",
      }),
    );
    const fetchMock = vi.fn();

    vi.stubGlobal("browser", {
      runtime: {
        sendMessage,
      },
    });
    vi.stubGlobal("fetch", fetchMock);

    const file = await assetToFile(asset);
    const text = new TextDecoder().decode(await file.arrayBuffer());

    expect(sendMessage).toHaveBeenCalledWith({
      type: "asset.download",
      asset,
    });
    expect(fetchMock).not.toHaveBeenCalled();
    expect(file.name).toBe("douyin-cover.png");
    expect(file.type).toBe("image/png");
    expect(text).toBe("Hello Douyin");
  });

  it("surfaces background download errors", async () => {
    vi.stubGlobal("browser", {
      runtime: {
        sendMessage: vi.fn(() =>
          Promise.resolve({
            error: "Asset download failed with HTTP 403.",
          }),
        ),
      },
    });

    await expect(assetToFile(asset)).rejects.toThrow(
      "Asset download failed with HTTP 403.",
    );
  });
});

function createDouyinPlatform(
  text = defaultArticleText,
): ExtensionPublishPlatformHandoff {
  return {
    platform: "douyin",
    adapter_key: "DYNAMIC_DOUYIN",
    inject_url:
      "https://creator.douyin.com/creator-micro/content/upload?default-tab=5",
    content_kind: "article",
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

function renderDouyinArticleEditor(): void {
  document.body.innerHTML = `
    <div class="semi-input-wrapper input-xXwC7n semi-input-wrapper__with-suffix semi-input-wrapper-default">
      <input class="semi-input semi-input-default" type="text" placeholder="${fullTitlePlaceholder}" value="">
      <div class="semi-input-suffix"><span class="limit-KYcfUi">0/30</span></div>
    </div>
    <div class="semi-input-wrapper input-uCtIZt semi-input-wrapper__with-suffix semi-input-wrapper-default">
      <input class="semi-input semi-input-default" type="text" placeholder="${fullSummaryPlaceholder}" value="">
      <div class="semi-input-suffix"><span class="limit-IbdQBn">0/30</span></div>
    </div>
    <div class="editor-DoqDrA">
      <div elementtiming="douyin_creator_content-element-timing" contenteditable="true" role="textbox" translate="no" class="tiptap ProseMirror" tabindex="0">
        <p data-placeholder="${bodyPlaceholder}" class="is-empty placeholder-RrZ1QM"><br class="ProseMirror-trailingBreak"></p>
      </div>
    </div>
  `;
}

describe("runDouyinDynamicAdapter", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
    window.history.replaceState(
      {},
      "",
      "https://creator.douyin.com/creator-micro/content/post/article?default-tab=5&enter_from=publish_page&media_type=article&type=new",
    );
  });

  it("fills the Douyin article title, summary, and body editor", async () => {
    renderDouyinArticleEditor();

    const result = await runDouyinDynamicAdapter(
      createDouyinPlatform(articleText),
      articleTitle,
    );

    expect(result.status).toBe("user_review");
    expect(
      document.querySelector<HTMLInputElement>(
        `input[placeholder*="${titlePlaceholder}"]`,
      )?.value,
    ).toBe(articleTitle.slice(0, 30));
    expect(
      document.querySelector<HTMLInputElement>(
        `input[placeholder*="${summaryPlaceholder}"]`,
      )?.value,
    ).toBe(articleSummary);
    expect(
      document.querySelector<HTMLElement>('[contenteditable="true"]')
        ?.textContent,
    ).toBe(articleText);
  });

  it("clicks the Douyin article AI image generation button after filling text", async () => {
    renderDouyinArticleEditor();
    let aiGenerateClicked = false;
    document.body.insertAdjacentHTML(
      "beforeend",
      `<div class="aiButton-x4dXs_"><span>${articleAiGenerateText}</span></div>`,
    );
    document
      .querySelector(".aiButton-x4dXs_")
      ?.addEventListener("click", () => {
        aiGenerateClicked = true;
      });

    const result = await runDouyinDynamicAdapter(
      createDouyinPlatform(articleText),
      articleTitle,
    );

    expect(result.status).toBe("user_review");
    expect(aiGenerateClicked).toBe(true);
    expect(result.metadata?.ai_image_generation_clicked).toBe(true);
  });

  it("clicks the upload page article button before filling the editor", async () => {
    window.history.replaceState(
      {},
      "",
      "https://creator.douyin.com/creator-micro/content/upload?default-tab=5",
    );
    document.body.innerHTML = `<button type="button">${articleButtonText}</button>`;
    document.querySelector("button")?.addEventListener("click", () => {
      window.history.pushState(
        {},
        "",
        "https://creator.douyin.com/creator-micro/content/post/article?default-tab=5&enter_from=publish_page&media_type=article&type=new",
      );
      renderDouyinArticleEditor();
    });

    const result = await runDouyinDynamicAdapter(
      createDouyinPlatform(shortBody),
      shortTitle,
    );

    expect(result.status).toBe("user_review");
    expect(
      document.querySelector<HTMLInputElement>(
        `input[placeholder*="${titlePlaceholder}"]`,
      )?.value,
    ).toBe(shortTitle);
  });

  it("waits for a delayed upload page article button before filling the editor", async () => {
    window.history.replaceState(
      {},
      "",
      "https://creator.douyin.com/creator-micro/content/upload?default-tab=5",
    );
    document.body.innerHTML = `<div class="semi-spin">Loading</div>`;
    window.setTimeout(() => {
      document.body.innerHTML = `<button type="button">${articleButtonText}</button>`;
      document.querySelector("button")?.addEventListener("click", () => {
        window.history.pushState(
          {},
          "",
          "https://creator.douyin.com/creator-micro/content/post/article?default-tab=5&enter_from=publish_page&media_type=article&type=new",
        );
        renderDouyinArticleEditor();
      });
    }, 0);

    const result = await runDouyinDynamicAdapter(
      createDouyinPlatform(shortBody),
      shortTitle,
    );

    expect(result.status).toBe("user_review");
    expect(
      document.querySelector<HTMLInputElement>(
        `input[placeholder*="${titlePlaceholder}"]`,
      )?.value,
    ).toBe(shortTitle);
  });

  it("waits until the article editor is open before attaching assets", async () => {
    window.history.replaceState(
      {},
      "",
      "https://creator.douyin.com/creator-micro/content/upload?default-tab=5",
    );
    let articleUploadClicked = false;
    document.body.innerHTML = `<button type="button">${articleButtonText}</button>`;
    document.querySelector("button")?.addEventListener("click", () => {
      window.history.pushState(
        {},
        "",
        "https://creator.douyin.com/creator-micro/content/post/article?default-tab=5&enter_from=publish_page&media_type=article&type=new",
      );
      renderDouyinArticleEditor();
      document.body.insertAdjacentHTML(
        "beforeend",
        `<div class="mycard-ixFFfp"><span>${articleImageUploadText}</span></div>`,
      );
      document
        .querySelector(".mycard-ixFFfp")
        ?.addEventListener("click", () => {
          articleUploadClicked = true;
          document.body.insertAdjacentHTML(
            "beforeend",
            `<input type="file" accept="image/png">`,
          );
        });
    });
    const sendMessage = vi.fn(() =>
      Promise.resolve({
        name: "article-image.png",
        mime_type: "image/png",
        data_base64: "SGVsbG8gRG91eWlu",
      }),
    );
    vi.stubGlobal("browser", {
      runtime: {
        sendMessage,
      },
    });
    vi.stubGlobal(
      "DataTransfer",
      class {
        private readonly filesInternal: File[] = [];

        readonly items = {
          add: (file: File) => {
            this.filesInternal.push(file);
          },
        };

        get files() {
          return this.filesInternal;
        }
      },
    );

    const platform = createDouyinPlatform(shortBody);
    platform.assets = [
      {
        type: "image",
        source_url: "https://assets.example.com/article-image.png",
        name: "article-image.png",
        mime_type: "image/png",
      },
    ];

    const result = await runDouyinDynamicAdapter(platform, shortTitle);

    expect(articleUploadClicked).toBe(true);
    expect(sendMessage).toHaveBeenCalledWith({
      type: "asset.download",
      asset: platform.assets[0],
    });
    expect(result.error_message).not.toBe(
      "Could not find the Douyin media upload input.",
    );
  });
});

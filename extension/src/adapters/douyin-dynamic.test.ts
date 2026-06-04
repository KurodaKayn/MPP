import { beforeEach, describe, expect, it, vi } from "vitest";
import { assetToFile, runDouyinDynamicAdapter } from "./douyin-dynamic";
import type {
  ExtensionPublishPlatformHandoff,
  HandoffAsset,
} from "../types/handoff";

const asset: HandoffAsset = {
  type: "image",
  source_url: "https://assets.example.com/douyin-cover.png",
  name: "douyin-cover.png",
  mime_type: "image/png",
};
const defaultArticleText =
  "\u8fd9\u662f\u4e00\u6bb5\u6296\u97f3\u6587\u7ae0\u6b63\u6587\u3002\n\u7b2c\u4e8c\u6bb5\u6b63\u6587\u3002";
const articleText =
  "\u7b2c\u4e00\u6bb5\u6b63\u6587\u5185\u5bb9\u4f1a\u8fdb\u5165\u6458\u8981\u3002\n\u7b2c\u4e8c\u6bb5\u6b63\u6587\u3002";
const articleSummary =
  "\u7b2c\u4e00\u6bb5\u6b63\u6587\u5185\u5bb9\u4f1a\u8fdb\u5165\u6458\u8981\u3002 \u7b2c\u4e8c\u6bb5\u6b63\u6587\u3002";
const articleTitle =
  "\u8fd9\u662f\u4e00\u4e2a\u8d85\u8fc7\u4e09\u5341\u4e2a\u5b57\u7684\u6587\u7ae0\u6807\u9898\u7528\u4e8e\u6d4b\u8bd5\u622a\u65ad\u903b\u8f91";
const shortBody = "\u6b63\u6587";
const shortTitle = "\u6587\u7ae0\u6807\u9898";
const articleButtonText = "\u6211\u8981\u53d1\u6587";
const articleImageUploadText = "\u70b9\u51fb\u4e0a\u4f20\u56fe\u7247";
const titlePlaceholder = "\u8bf7\u8f93\u5165\u6587\u7ae0\u6807\u9898";
const fullTitlePlaceholder = `${titlePlaceholder}\uff0c\u6700\u591a\u4e0d\u8d85\u8fc730\u4e2a\u5b57`;
const summaryPlaceholder = "\u6dfb\u52a0\u5185\u5bb9\u6458\u8981";
const fullSummaryPlaceholder = `${summaryPlaceholder}\u6216\u6587\u7ae0\u7cbe\u5f69\u90e8\u5206\u5438\u5f15\u7528\u6237\u9605\u8bfb\uff0c\u6700\u591a\u4e0d\u8d85\u8fc730\u4e2a\u5b57`;
const bodyPlaceholder = "\u8bf7\u8f93\u5165\u6b63\u6587";

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

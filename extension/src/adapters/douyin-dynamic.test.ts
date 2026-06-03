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
  text = "这是一段抖音文章正文。\n第二段正文。",
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
      <input class="semi-input semi-input-default" type="text" placeholder="请输入文章标题，最多不超过30个字" value="">
      <div class="semi-input-suffix"><span class="limit-KYcfUi">0/30</span></div>
    </div>
    <div class="semi-input-wrapper input-uCtIZt semi-input-wrapper__with-suffix semi-input-wrapper-default">
      <input class="semi-input semi-input-default" type="text" placeholder="添加内容摘要或文章精彩部分吸引用户阅读，最多不超过30个字" value="">
      <div class="semi-input-suffix"><span class="limit-IbdQBn">0/30</span></div>
    </div>
    <div class="editor-DoqDrA">
      <div elementtiming="douyin_creator_content-element-timing" contenteditable="true" role="textbox" translate="no" class="tiptap ProseMirror" tabindex="0">
        <p data-placeholder="请输入正文" class="is-empty placeholder-RrZ1QM"><br class="ProseMirror-trailingBreak"></p>
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
      createDouyinPlatform("第一段正文内容会进入摘要。\n第二段正文。"),
      "这是一个超过三十个字的文章标题用于测试截断逻辑",
    );

    expect(result.status).toBe("user_review");
    expect(
      document.querySelector<HTMLInputElement>(
        'input[placeholder*="请输入文章标题"]',
      )?.value,
    ).toBe("这是一个超过三十个字的文章标题用于测试截断逻辑".slice(0, 30));
    expect(
      document.querySelector<HTMLInputElement>(
        'input[placeholder*="添加内容摘要"]',
      )?.value,
    ).toBe("第一段正文内容会进入摘要。 第二段正文。");
    expect(
      document.querySelector<HTMLElement>('[contenteditable="true"]')
        ?.textContent,
    ).toBe("第一段正文内容会进入摘要。\n第二段正文。");
  });

  it("clicks the upload page article button before filling the editor", async () => {
    window.history.replaceState(
      {},
      "",
      "https://creator.douyin.com/creator-micro/content/upload?default-tab=5",
    );
    document.body.innerHTML = `<button type="button">我要发文</button>`;
    document.querySelector("button")?.addEventListener("click", () => {
      window.history.pushState(
        {},
        "",
        "https://creator.douyin.com/creator-micro/content/post/article?default-tab=5&enter_from=publish_page&media_type=article&type=new",
      );
      renderDouyinArticleEditor();
    });

    const result = await runDouyinDynamicAdapter(
      createDouyinPlatform("正文"),
      "文章标题",
    );

    expect(result.status).toBe("user_review");
    expect(
      document.querySelector<HTMLInputElement>(
        'input[placeholder*="请输入文章标题"]',
      )?.value,
    ).toBe("文章标题");
  });

  it("waits until the article editor is open before attaching assets", async () => {
    window.history.replaceState(
      {},
      "",
      "https://creator.douyin.com/creator-micro/content/upload?default-tab=5",
    );
    let articleUploadClicked = false;
    document.body.innerHTML = `<button type="button">我要发文</button>`;
    document.querySelector("button")?.addEventListener("click", () => {
      window.history.pushState(
        {},
        "",
        "https://creator.douyin.com/creator-micro/content/post/article?default-tab=5&enter_from=publish_page&media_type=article&type=new",
      );
      renderDouyinArticleEditor();
      document.body.insertAdjacentHTML(
        "beforeend",
        `<div class="mycard-ixFFfp"><span>点击上传图片</span></div>`,
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

    const platform = createDouyinPlatform("正文");
    platform.assets = [
      {
        type: "image",
        source_url: "https://assets.example.com/article-image.png",
        name: "article-image.png",
        mime_type: "image/png",
      },
    ];

    const result = await runDouyinDynamicAdapter(platform, "文章标题");

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

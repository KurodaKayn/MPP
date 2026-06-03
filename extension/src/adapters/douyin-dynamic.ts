import { detectGenericCreatorAccount } from "../account/detectors";
import type {
  ExtensionPublishPlatformHandoff,
  HandoffAsset,
} from "../types/handoff";
import type {
  AssetDownloadResponse,
  BackgroundMessage,
} from "../types/messages";
import type { AdapterResult } from "./shared";
import {
  failed,
  fillTextTarget,
  findFirstElement,
  getDraftText,
  isOnExpectedHost,
  userReview,
} from "./shared";

const TEXT_TARGET_SELECTORS = [
  '.ProseMirror[contenteditable="true"]',
  '[role="textbox"][contenteditable="true"]',
  '[data-placeholder="请输入正文"]',
  '[contenteditable="true"]',
  'textarea[placeholder*="描述"]',
  'textarea[placeholder*="标题"]',
  'textarea[placeholder*="文案"]',
  "textarea",
];
const ARTICLE_TITLE_SELECTORS = [
  'input[placeholder*="请输入文章标题"]',
  'input[placeholder*="文章标题"]',
];
const ARTICLE_SUMMARY_SELECTORS = [
  'input[placeholder*="添加内容摘要"]',
  'input[placeholder*="文章精彩部分"]',
];
const FILE_INPUT_SELECTORS = [
  'input[type="file"][accept*="video"]',
  'input[type="file"][accept*="image"]',
  'input[type="file"]',
];
const ARTICLE_BUTTON_TEXT = "我要发文";
const ARTICLE_IMAGE_UPLOAD_TEXT = "点击上传图片";
const DOUYIN_ARTICLE_PATH = "/creator-micro/content/post/article";
const ELEMENT_WAIT_TIMEOUT_MS = 10_000;
const ELEMENT_WAIT_INTERVAL_MS = 250;

function wait(milliseconds: number): Promise<void> {
  return new Promise((resolve) => {
    globalThis.setTimeout(resolve, milliseconds);
  });
}

async function waitForFirstElement<T extends Element>(
  selectors: string[],
  timeoutMs = ELEMENT_WAIT_TIMEOUT_MS,
): Promise<T | null> {
  const startedAt = Date.now();

  while (Date.now() - startedAt < timeoutMs) {
    const element = findFirstElement<T>(selectors);

    if (element) {
      return element;
    }

    await wait(ELEMENT_WAIT_INTERVAL_MS);
  }

  return findFirstElement<T>(selectors);
}

async function waitForCondition(
  condition: () => boolean,
  timeoutMs = ELEMENT_WAIT_TIMEOUT_MS,
): Promise<boolean> {
  const startedAt = Date.now();

  while (Date.now() - startedAt < timeoutMs) {
    if (condition()) {
      return true;
    }

    await wait(ELEMENT_WAIT_INTERVAL_MS);
  }

  return condition();
}

function findButtonByText(text: string): HTMLButtonElement | null {
  const buttons = Array.from(document.querySelectorAll("button"));

  return (
    buttons.find((button) => button.textContent?.trim().includes(text)) ?? null
  );
}

function findElementByText<T extends HTMLElement>(
  selectors: string[],
  text: string,
): T | null {
  const elements = selectors.flatMap((selector) =>
    Array.from(document.querySelectorAll<T>(selector)),
  );

  return (
    elements.find((element) => element.textContent?.trim().includes(text)) ??
    null
  );
}

function isOnDouyinArticlePage(): boolean {
  return location.pathname.replace(/\/$/, "") === DOUYIN_ARTICLE_PATH;
}

async function enterArticleEditor(): Promise<boolean> {
  if (isOnDouyinArticlePage()) {
    return true;
  }

  const articleButton = findButtonByText(ARTICLE_BUTTON_TEXT);

  if (!articleButton) {
    return false;
  }

  articleButton.click();

  return waitForCondition(isOnDouyinArticlePage);
}

function isAssetDownloadResponse(
  value: unknown,
): value is AssetDownloadResponse {
  return (
    typeof value === "object" &&
    value !== null &&
    "name" in value &&
    "mime_type" in value &&
    "data_base64" in value &&
    typeof value.name === "string" &&
    typeof value.mime_type === "string" &&
    typeof value.data_base64 === "string"
  );
}

function base64ToUint8Array(value: string): Uint8Array {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);

  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }

  return bytes;
}

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  const buffer = new ArrayBuffer(bytes.byteLength);
  new Uint8Array(buffer).set(bytes);
  return buffer;
}

async function downloadAsset(
  asset: HandoffAsset,
): Promise<AssetDownloadResponse> {
  const message: BackgroundMessage = {
    type: "asset.download",
    asset,
  };
  const response = await browser.runtime.sendMessage(message);

  if (isAssetDownloadResponse(response)) {
    return response;
  }

  if (
    typeof response === "object" &&
    response !== null &&
    "error" in response &&
    typeof response.error === "string"
  ) {
    throw new Error(response.error);
  }

  throw new Error(`Failed to download ${asset.name}.`);
}

export async function assetToFile(asset: HandoffAsset): Promise<File> {
  const downloadedAsset = await downloadAsset(asset);
  const bytes = base64ToUint8Array(downloadedAsset.data_base64);

  return new File([toArrayBuffer(bytes)], downloadedAsset.name, {
    type: downloadedAsset.mime_type,
  });
}

async function buildAssetFileList(assets: HandoffAsset[]): Promise<FileList> {
  const dataTransfer = new DataTransfer();

  for (const asset of assets) {
    dataTransfer.items.add(await assetToFile(asset));
  }

  return dataTransfer.files;
}

async function uploadAssets(assets: HandoffAsset[]): Promise<number> {
  if (assets.length === 0) {
    return 0;
  }

  const input =
    await waitForFirstElement<HTMLInputElement>(FILE_INPUT_SELECTORS);

  if (!input) {
    throw new Error("Could not find the Douyin media upload input.");
  }

  if (!input.multiple && assets.length > 1) {
    throw new Error("The Douyin upload input accepts only one media file.");
  }

  input.files = await buildAssetFileList(assets);
  input.dispatchEvent(new Event("input", { bubbles: true }));
  input.dispatchEvent(new Event("change", { bubbles: true }));

  return assets.length;
}

async function uploadArticleAssets(assets: HandoffAsset[]): Promise<number> {
  if (assets.length === 0) {
    return 0;
  }

  if (!findFirstElement<HTMLInputElement>(FILE_INPUT_SELECTORS)) {
    const uploadTrigger = findElementByText<HTMLElement>(
      ["button", '[class*="mycard"]'],
      ARTICLE_IMAGE_UPLOAD_TEXT,
    );

    uploadTrigger?.click();
  }

  return uploadAssets(assets);
}

function limitDouyinArticleText(value: string): string {
  return value.trim().slice(0, 30);
}

function createArticleSummary(text: string): string {
  return limitDouyinArticleText(text.replace(/\s+/g, " "));
}

async function fillDouyinArticleEditor(
  platform: ExtensionPublishPlatformHandoff,
  projectTitle: string,
): Promise<void> {
  const titleTarget = await waitForFirstElement<HTMLInputElement>(
    ARTICLE_TITLE_SELECTORS,
  );
  const summaryTarget = await waitForFirstElement<HTMLInputElement>(
    ARTICLE_SUMMARY_SELECTORS,
  );
  const bodyTarget = await waitForFirstElement<
    HTMLElement | HTMLTextAreaElement
  >(TEXT_TARGET_SELECTORS);

  if (!titleTarget || !summaryTarget || !bodyTarget) {
    throw new Error("Could not find the Douyin article editor.");
  }

  const draftText = getDraftText(platform);

  fillTextTarget(titleTarget, limitDouyinArticleText(projectTitle));
  fillTextTarget(summaryTarget, createArticleSummary(draftText));
  fillTextTarget(bodyTarget, draftText);
}

export async function runDouyinDynamicAdapter(
  platform: ExtensionPublishPlatformHandoff,
  projectTitle: string,
): Promise<AdapterResult> {
  if (!isOnExpectedHost(["creator.douyin.com"])) {
    return failed("Douyin adapter can only run on Douyin creator pages.");
  }

  const account = detectGenericCreatorAccount();

  if (account.status === "signed_out") {
    return failed(
      "Please sign in to Douyin before publishing.",
      account.reason,
    );
  }

  if (platform.content_kind === "article") {
    const articleEditorReady = await enterArticleEditor();

    if (!articleEditorReady) {
      return failed("Could not open the Douyin article editor.");
    }

    try {
      await fillDouyinArticleEditor(platform, projectTitle);
    } catch (error) {
      return failed("Could not find the Douyin editor.", error);
    }

    let uploadedAssets = 0;

    try {
      uploadedAssets = await uploadArticleAssets(platform.assets);
    } catch (error) {
      return failed("Could not attach Douyin media assets.", error);
    }

    return userReview("Article draft prepared. Review platform settings.", {
      account_status: account.status,
      assets: platform.assets.length,
      assets_uploaded: uploadedAssets,
      asset_types: [...new Set(platform.assets.map((asset) => asset.type))],
      auto_publish: false,
      content_kind: platform.content_kind,
    });
  }

  let uploadedAssets = 0;

  try {
    uploadedAssets = await uploadAssets(platform.assets);
  } catch (error) {
    return failed("Could not attach Douyin media assets.", error);
  }

  const bodyTarget = await waitForFirstElement<
    HTMLElement | HTMLTextAreaElement
  >(TEXT_TARGET_SELECTORS);

  if (!bodyTarget) {
    return failed("Could not find the Douyin editor.");
  }

  fillTextTarget(bodyTarget, getDraftText(platform));

  return userReview(
    "Draft text prepared. Review media and platform settings.",
    {
      account_status: account.status,
      assets: platform.assets.length,
      assets_uploaded: uploadedAssets,
      asset_types: [...new Set(platform.assets.map((asset) => asset.type))],
      auto_publish: false,
    },
  );
}

import { detectGenericCreatorAccount } from "../account/detectors";
import type {
  ExtensionPublishPlatformHandoff,
  HandoffAsset,
} from "../types/handoff";
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
  '[contenteditable="true"]',
  'textarea[placeholder*="描述"]',
  'textarea[placeholder*="标题"]',
  'textarea[placeholder*="文案"]',
  "textarea",
];
const FILE_INPUT_SELECTORS = [
  'input[type="file"][accept*="video"]',
  'input[type="file"][accept*="image"]',
  'input[type="file"]',
];
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

async function assetToFile(asset: HandoffAsset): Promise<File> {
  const response = await fetch(asset.source_url);

  if (!response.ok) {
    throw new Error(`Failed to download ${asset.name}.`);
  }

  const blob = await response.blob();
  const type = asset.mime_type || blob.type;

  return new File([blob], asset.name, { type });
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

export async function runDouyinDynamicAdapter(
  platform: ExtensionPublishPlatformHandoff,
  _projectTitle: string,
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

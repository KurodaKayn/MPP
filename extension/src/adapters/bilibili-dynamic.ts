import { detectGenericCreatorAccount } from "../account/detectors";
import type { ExtensionPublishPlatformHandoff } from "../types/handoff";
import type { AdapterResult } from "./shared";
import {
  failed,
  fillTextTarget,
  findFirstElement,
  getDraftText,
  isOnExpectedHost,
  userReview,
} from "./shared";
import zhCn from "../i18n/zh-CN.json";

const { bilibili } = zhCn.adapters;

export async function runBilibiliDynamicAdapter(
  platform: ExtensionPublishPlatformHandoff,
  _projectTitle: string,
): Promise<AdapterResult> {
  if (!isOnExpectedHost(["bilibili.com"])) {
    return failed("Bilibili adapter can only run on Bilibili pages.");
  }

  const account = detectGenericCreatorAccount();

  if (account.status === "signed_out") {
    return failed(
      "Please sign in to Bilibili before publishing.",
      account.reason,
    );
  }

  const bodyTarget = findFirstElement<HTMLElement | HTMLTextAreaElement>([
    '[contenteditable="true"]',
    `textarea[placeholder*="${bilibili.dynamicPlaceholder}"]`,
    "textarea",
  ]);

  if (!bodyTarget) {
    return failed("Could not find the Bilibili dynamic editor.");
  }

  fillTextTarget(bodyTarget, getDraftText(platform));

  return userReview("Dynamic text prepared. Waiting for user review.", {
    account_status: account.status,
    assets: platform.assets.length,
    auto_publish: false,
  });
}

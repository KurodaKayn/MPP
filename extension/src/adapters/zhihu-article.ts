import { detectZhihuAccount } from "../account/detectors";
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

const { zhihu } = zhCn.adapters;

export async function runZhihuArticleAdapter(
  platform: ExtensionPublishPlatformHandoff,
  projectTitle: string,
): Promise<AdapterResult> {
  if (!isOnExpectedHost(["zhuanlan.zhihu.com", "zhihu.com"])) {
    return failed("Zhihu adapter can only run on Zhihu editor pages.");
  }

  const account = detectZhihuAccount();

  if (account.status === "signed_out") {
    return failed("Please sign in to Zhihu before publishing.", account.reason);
  }

  const titleTarget = findFirstElement<HTMLInputElement | HTMLTextAreaElement>([
    `textarea[placeholder*="${zhihu.titlePlaceholder}"]`,
    `input[placeholder*="${zhihu.titlePlaceholder}"]`,
    `[contenteditable="true"][data-placeholder*="${zhihu.titlePlaceholder}"]`,
  ]);
  const bodyTarget = findFirstElement<HTMLElement | HTMLTextAreaElement>([
    `[contenteditable="true"][data-placeholder*="${zhihu.bodyPlaceholder}"]`,
    `[contenteditable="true"][aria-label*="${zhihu.bodyPlaceholder}"]`,
    '[contenteditable="true"]',
    "textarea",
  ]);

  if (!bodyTarget) {
    return failed("Could not find the Zhihu article editor.");
  }

  const draftText = getDraftText(platform);

  if (titleTarget) {
    fillTextTarget(titleTarget, projectTitle);
  }

  fillTextTarget(bodyTarget, draftText);

  return userReview("Draft filled. Waiting for user review.", {
    account_status: account.status,
    assets: platform.assets.length,
    auto_publish: false,
  });
}

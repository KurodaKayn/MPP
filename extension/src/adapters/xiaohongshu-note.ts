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

const { xiaohongshu } = zhCn.adapters;

export async function runXiaohongshuNoteAdapter(
  platform: ExtensionPublishPlatformHandoff,
  _projectTitle: string,
): Promise<AdapterResult> {
  if (!isOnExpectedHost(["creator.xiaohongshu.com"])) {
    return failed(
      "Xiaohongshu adapter can only run on Xiaohongshu creator pages.",
    );
  }

  const account = detectGenericCreatorAccount();

  if (account.status === "signed_out") {
    return failed(
      "Please sign in to Xiaohongshu before publishing.",
      account.reason,
    );
  }

  const bodyTarget = findFirstElement<HTMLElement | HTMLTextAreaElement>([
    '[contenteditable="true"]',
    `textarea[placeholder*="${xiaohongshu.descriptionPlaceholder}"]`,
    "textarea",
  ]);

  if (!bodyTarget) {
    return failed("Could not find the Xiaohongshu note editor.");
  }

  fillTextTarget(bodyTarget, getDraftText(platform));

  return userReview(
    "Note text prepared. Upload assets and review before publishing.",
    {
      account_status: account.status,
      assets: platform.assets.length,
      auto_publish: false,
    },
  );
}

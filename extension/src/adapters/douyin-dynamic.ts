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

  const bodyTarget = findFirstElement<HTMLElement | HTMLTextAreaElement>([
    '[contenteditable="true"]',
    'textarea[placeholder*="描述"]',
    "textarea",
  ]);

  if (!bodyTarget) {
    return failed("Could not find the Douyin editor.");
  }

  fillTextTarget(bodyTarget, getDraftText(platform));

  return userReview(
    "Draft text prepared. Review media and platform settings.",
    {
      account_status: account.status,
      assets: platform.assets.length,
      auto_publish: false,
    },
  );
}

import { describe, expect, it } from "vitest";
import {
  getCapabilityByAdapterKey,
  isCapabilityInjectUrl,
  isSupportedAdapterKey,
} from "./capabilities";

describe("isCapabilityInjectUrl", () => {
  it("allows Douyin upload and article publishing pages", () => {
    expect(
      isCapabilityInjectUrl(
        "DYNAMIC_DOUYIN",
        "https://creator.douyin.com/creator-micro/content/upload?default-tab=5",
      ),
    ).toBe(true);
    expect(
      isCapabilityInjectUrl(
        "DYNAMIC_DOUYIN",
        "https://creator.douyin.com/creator-micro/content/post/article?default-tab=5&enter_from=publish_page&media_type=article&type=new",
      ),
    ).toBe(true);
  });

  it("supports X compose post handoffs", () => {
    expect(isSupportedAdapterKey("POST_X")).toBe(true);

    const capability = getCapabilityByAdapterKey("POST_X");

    expect(capability).toMatchObject({
      platform: "x",
      adapter_key: "POST_X",
      inject_url: "https://x.com/compose/post",
      content_kinds: ["dynamic_post"],
      target_formats: ["text"],
      requires_review: true,
      auto_publish_allowed: false,
    });
    expect(
      isCapabilityInjectUrl("POST_X", "https://x.com/compose/post?text=draft"),
    ).toBe(true);
  });
});

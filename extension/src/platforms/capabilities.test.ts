import { describe, expect, it } from "vitest";
import { isCapabilityInjectUrl } from "./capabilities";

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
});

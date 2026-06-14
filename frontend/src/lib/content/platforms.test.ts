import { describe, expect, it } from "vitest";
import {
  AUTO_PUBLISH_PLATFORM_TABS,
  PLATFORM_TABS,
  getPlatformDefaultLabel,
} from "./platforms";

describe("platform capability tabs", () => {
  it("exposes project-selectable platforms from the generated capability contract", () => {
    expect(PLATFORM_TABS.map((platform) => platform.value)).toEqual([
      "wechat",
      "zhihu",
      "x",
      "douyin",
    ]);
  });

  it("derives automatic publish tabs from platform capabilities", () => {
    expect(
      AUTO_PUBLISH_PLATFORM_TABS.map((platform) => platform.value),
    ).toEqual(["wechat", "zhihu", "x"]);
  });

  it("resolves default labels from platform capabilities", () => {
    expect(getPlatformDefaultLabel("zhihu")).toBe("Zhihu");
    expect(getPlatformDefaultLabel("unknown")).toBe("unknown");
  });
});

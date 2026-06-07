import { describe, expect, it } from "vitest";
import {
  PLATFORM_UI_CONFIGS,
  getImplementedHandoffPlatforms,
  getPlatformUiConfig,
  isPlatformHandoffEnabled,
} from "./platform-ui";

describe("platform UI config", () => {
  it("defines the four selectable platform surfaces", () => {
    expect(PLATFORM_UI_CONFIGS.map((platform) => platform.key)).toEqual([
      "douyin",
      "wechat",
      "x",
      "zhihu",
    ]);
  });

  it("uses public platform icon assets for every platform", () => {
    expect(PLATFORM_UI_CONFIGS.map((platform) => platform.iconPath)).toEqual([
      "/icon/platforms/douyin.svg",
      "/icon/platforms/wechat.svg",
      "/icon/platforms/x.svg",
      "/icon/platforms/zhihu.svg",
    ]);
  });

  it("marks only Douyin as connected to the handoff flow", () => {
    expect(getPlatformUiConfig("douyin")).toMatchObject({
      handoffPlatform: "douyin",
      implementationStatus: "implemented",
      statusLabel: "Ready",
    });
    expect(isPlatformHandoffEnabled("douyin")).toBe(true);
    expect(getImplementedHandoffPlatforms()).toEqual(["douyin"]);

    for (const platform of ["wechat", "x", "zhihu"] as const) {
      expect(getPlatformUiConfig(platform)).toMatchObject({
        implementationStatus: "ui_only",
        statusLabel: "Coming soon",
      });
      expect(isPlatformHandoffEnabled(platform)).toBe(false);
    }
  });
});

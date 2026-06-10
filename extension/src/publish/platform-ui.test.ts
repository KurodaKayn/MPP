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

  it("marks Douyin and X as connected to the handoff flow", () => {
    expect(getPlatformUiConfig("douyin")).toMatchObject({
      handoffPlatform: "douyin",
      implementationStatus: "implemented",
      statusLabel: "Ready",
    });
    expect(getPlatformUiConfig("x")).toMatchObject({
      handoffPlatform: "x",
      implementationStatus: "implemented",
      statusLabel: "Ready",
    });
    expect(isPlatformHandoffEnabled("douyin")).toBe(true);
    expect(isPlatformHandoffEnabled("x")).toBe(true);
    expect(getImplementedHandoffPlatforms()).toEqual(["douyin", "x"]);

    for (const platform of ["wechat", "zhihu"] as const) {
      expect(getPlatformUiConfig(platform)).toMatchObject({
        implementationStatus: "ui_only",
        statusLabel: "Coming soon",
      });
      expect(isPlatformHandoffEnabled(platform)).toBe(false);
    }
  });
});

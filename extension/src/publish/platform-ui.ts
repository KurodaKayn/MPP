import type { PlatformKey } from "../types/platform";

export type PlatformUiKey = "douyin" | "wechat" | "x" | "zhihu";

export type PlatformImplementationStatus = "implemented" | "ui_only";

export interface PlatformUiConfig {
  key: PlatformUiKey;
  label: string;
  shortLabel: string;
  description: string;
  iconPath: string;
  implementationStatus: PlatformImplementationStatus;
  statusLabel: string;
  handoffPlatform?: PlatformKey;
}

export const PLATFORM_UI_CONFIGS: readonly PlatformUiConfig[] = [
  {
    key: "douyin",
    label: "Douyin",
    shortLabel: "Douyin",
    description: "Prepare article drafts in the Douyin creator editor.",
    iconPath: "/icon/platforms/douyin.svg",
    implementationStatus: "implemented",
    statusLabel: "Ready",
    handoffPlatform: "douyin",
  },
  {
    key: "wechat",
    label: "WeChat",
    shortLabel: "WeChat",
    description:
      "Platform selection UI is available before publishing support.",
    iconPath: "/icon/platforms/wechat.svg",
    implementationStatus: "ui_only",
    statusLabel: "Coming soon",
  },
  {
    key: "x",
    label: "X",
    shortLabel: "X",
    description:
      "Platform selection UI is available before publishing support.",
    iconPath: "/icon/platforms/x.svg",
    implementationStatus: "ui_only",
    statusLabel: "Coming soon",
  },
  {
    key: "zhihu",
    label: "Zhihu",
    shortLabel: "Zhihu",
    description:
      "Platform selection UI is available before publishing support.",
    iconPath: "/icon/platforms/zhihu.svg",
    implementationStatus: "ui_only",
    statusLabel: "Coming soon",
  },
];

export function getPlatformUiConfig(key: PlatformUiKey): PlatformUiConfig {
  const config = PLATFORM_UI_CONFIGS.find((platform) => platform.key === key);

  if (!config) {
    throw new Error(`Unsupported platform UI key: ${key}`);
  }

  return config;
}

export function isPlatformHandoffEnabled(key: PlatformUiKey): boolean {
  return Boolean(getPlatformUiConfig(key).handoffPlatform);
}

export function getImplementedHandoffPlatforms(): PlatformKey[] {
  return PLATFORM_UI_CONFIGS.flatMap((platform) =>
    platform.handoffPlatform ? [platform.handoffPlatform] : [],
  );
}

export const PLATFORM_TABS = [
  {
    value: "wechat",
    label: "platforms.wechat",
    defaultLabel: "WeChat",
    icon: "/icons/platforms/wechat.svg",
  },
  {
    value: "zhihu",
    label: "platforms.zhihu",
    defaultLabel: "Zhihu",
    icon: "/icons/platforms/zhihu.svg",
  },
  {
    value: "x",
    label: "platforms.x",
    defaultLabel: "X",
    icon: "/icons/platforms/x.svg",
  },
  {
    value: "douyin",
    label: "platforms.douyin",
    defaultLabel: "Douyin",
    icon: "/icons/platforms/douyin.svg",
  },
] as const;

export type PlatformTab = (typeof PLATFORM_TABS)[number];

export function getPlatformDefaultLabel(platform: string) {
  return (
    PLATFORM_TABS.find((item) => item.value === platform)?.defaultLabel ??
    platform
  );
}

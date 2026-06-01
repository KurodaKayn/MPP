export const PLATFORM_TABS = [
  {
    value: "wechat",
    label: "platforms.wechat",
    icon: "/icons/platforms/wechat.svg",
  },
  {
    value: "zhihu",
    label: "platforms.zhihu",
    icon: "/icons/platforms/zhihu.svg",
  },
  {
    value: "x",
    label: "platforms.x",
    icon: "/icons/platforms/x.svg",
  },
  {
    value: "bilibili",
    label: "platforms.bilibili",
    icon: "/icons/platforms/bilibili.svg",
  },
  {
    value: "xiaohongshu",
    label: "platforms.xiaohongshu",
    icon: "/icons/platforms/xiaohongshu.svg",
  },
] as const;

export type PlatformTab = (typeof PLATFORM_TABS)[number];

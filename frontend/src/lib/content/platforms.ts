export const PLATFORM_TABS = [
  {
    value: "wechat",
    label: "公众号",
    icon: "/icons/platforms/wechat.svg",
  },
  {
    value: "zhihu",
    label: "知乎",
    icon: "/icons/platforms/zhihu.svg",
  },
  {
    value: "x",
    label: "X",
    icon: "/icons/platforms/x.svg",
  },
  {
    value: "douyin",
    label: "抖音",
    icon: "/icons/platforms/douyin.svg",
  },
] as const;

export type PlatformTab = (typeof PLATFORM_TABS)[number];

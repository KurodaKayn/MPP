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
    value: "bilibili",
    label: "B站",
    icon: "/icons/platforms/bilibili.svg",
  },
  {
    value: "xiaohongshu",
    label: "小红书",
    icon: "/icons/platforms/xiaohongshu.svg",
  },
] as const;

export type PlatformTab = (typeof PLATFORM_TABS)[number];

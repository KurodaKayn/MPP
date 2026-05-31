const fallbackSiteUrl = "http://127.0.0.1:3000";

function normalizeSiteUrl(value: string | undefined) {
  try {
    const url = new URL(value ?? fallbackSiteUrl);
    const pathname = url.pathname.replace(/\/$/, "");

    return `${url.origin}${pathname}`;
  } catch {
    return fallbackSiteUrl;
  }
}

export const siteConfig = {
  name: "multi-platform poster",
  shortName: "MPP",
  url: normalizeSiteUrl(
    process.env.FRONTEND_BASE_URL ?? process.env.NEXT_PUBLIC_SITE_URL,
  ),
  title: "multi-platform poster | 多平台内容发布工作台",
  description:
    "multi-platform poster 是面向创作者和运营团队的多平台内容发布工作台，支持内容项目管理、平台草稿适配、发布状态追踪和 AI 辅助编辑。",
  keywords: [
    "多平台发布",
    "内容发布工具",
    "自媒体运营",
    "社交媒体发布",
    "AI 内容编辑",
    "公众号发布",
    "知乎发布",
    "抖音发布",
  ],
  indexableRoutes: [
    {
      path: "/",
      changeFrequency: "weekly",
      priority: 1,
    },
  ],
} as const;

export function absoluteUrl(path = "/") {
  return new URL(path, `${siteConfig.url}/`).toString();
}

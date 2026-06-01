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
  title: "multi-platform poster | Content Publishing Workbench",
  description:
    "multi-platform poster is a workbench for creators and content teams, supporting project management, draft adaptation, status tracking, and AI-assisted editing.",
  keywords: [
    "multi-platform publishing",
    "content tools",
    "social media management",
    "AI editing",
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

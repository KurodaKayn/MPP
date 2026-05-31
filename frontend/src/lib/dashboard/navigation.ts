export const dashboardRoutes = {
  auth: {
    title: "平台授权",
    url: "/dashboard/auth",
  },
  content: {
    title: "内容创作",
    url: "/dashboard/content",
  },
  overview: {
    title: "概览",
    url: "/dashboard",
  },
  posts: {
    title: "我的内容",
    url: "/dashboard/posts",
  },
  settings: {
    title: "设置",
    url: "/dashboard/settings",
  },
} as const;

export const dashboardMainNavItems = [
  dashboardRoutes.overview,
  dashboardRoutes.content,
  dashboardRoutes.posts,
  dashboardRoutes.auth,
] as const;

export function getDashboardPageTitle(pathname: string) {
  if (pathname === dashboardRoutes.overview.url) {
    return dashboardRoutes.overview.title;
  }

  const matchedRoute = Object.values(dashboardRoutes)
    .filter((route) => route.url !== dashboardRoutes.overview.url)
    .find(
      (route) => pathname === route.url || pathname.startsWith(`${route.url}/`),
    );

  return matchedRoute?.title ?? dashboardRoutes.overview.title;
}

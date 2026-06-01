export const dashboardRoutes = {
  auth: {
    title: "nav.auth",
    url: "/dashboard/auth",
  },
  content: {
    title: "nav.content",
    url: "/dashboard/content",
  },
  overview: {
    title: "nav.overview",
    url: "/dashboard",
  },
  posts: {
    title: "nav.posts",
    url: "/dashboard/posts",
  },
  settings: {
    title: "nav.settings",
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

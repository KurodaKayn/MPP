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
  const normalizedPathname = pathname === "" ? "/" : pathname;
  const unlocalizedPathname = normalizedPathname.replace(/^\/[^/]+/, "");
  const candidatePathnames = [
    normalizedPathname,
    unlocalizedPathname === "" ? "/" : unlocalizedPathname,
  ];

  if (candidatePathnames.includes(dashboardRoutes.overview.url)) {
    return dashboardRoutes.overview.title;
  }

  const matchedRoute = Object.values(dashboardRoutes)
    .filter((route) => route.url !== dashboardRoutes.overview.url)
    .find(
      (route) =>
        candidatePathnames.includes(route.url) ||
        candidatePathnames.some((candidate) =>
          candidate.startsWith(`${route.url}/`),
        ),
    );

  return matchedRoute?.title ?? dashboardRoutes.overview.title;
}

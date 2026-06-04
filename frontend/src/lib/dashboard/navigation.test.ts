import { describe, expect, it } from "vitest";
import { getDashboardPageTitle } from "./navigation";

describe("dashboard navigation", () => {
  it.each([
    ["/dashboard", "nav.overview"],
    ["/zh/dashboard", "nav.overview"],
    ["/dashboard/content", "nav.content"],
    ["/zh/dashboard/content", "nav.content"],
    ["/dashboard/content/project-1", "nav.content"],
    ["/dashboard/collab", "nav.collab"],
    ["/zh/dashboard/collab", "nav.collab"],
    ["/dashboard/posts", "nav.posts"],
    ["/dashboard/settings", "nav.settings"],
  ])("returns the page title for %s", (pathname, title) => {
    expect(getDashboardPageTitle(pathname)).toBe(title);
  });
});

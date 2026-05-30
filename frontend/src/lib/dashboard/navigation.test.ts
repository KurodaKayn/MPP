import { describe, expect, it } from "vitest";
import { getDashboardPageTitle } from "./navigation";

describe("dashboard navigation", () => {
  it.each([
    ["/dashboard", "概览"],
    ["/dashboard/content", "内容创作"],
    ["/dashboard/content/project-1", "内容创作"],
    ["/dashboard/posts", "我的内容"],
    ["/dashboard/settings", "设置"],
  ])("returns the page title for %s", (pathname, title) => {
    expect(getDashboardPageTitle(pathname)).toBe(title);
  });
});

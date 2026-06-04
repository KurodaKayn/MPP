import { describe, expect, it } from "vitest";
import type { NextRequest } from "next/server";
import { config, proxy } from "./proxy";
import { cookieName } from "./lib/i18n/settings";

function createRequest({
  cookies = {},
  headers = {},
  url,
}: {
  cookies?: Record<string, string>;
  headers?: Record<string, string>;
  url: string;
}) {
  const cookieStore = new Map(Object.entries(cookies));

  return {
    cookies: {
      get: (name: string) => {
        const value = cookieStore.get(name);
        return value ? { name, value } : undefined;
      },
      has: (name: string) => cookieStore.has(name),
    },
    headers: new Headers(headers),
    nextUrl: new URL(url),
    url,
  } as unknown as NextRequest;
}

describe("locale proxy", () => {
  it("excludes root metadata routes from locale redirects", () => {
    const matcher = config.matcher[0];

    expect(matcher).toContain("robots\\.txt");
    expect(matcher).toContain("sitemap\\.xml");
  });

  it("uses the referer locale when redirecting an unprefixed internal path", () => {
    const response = proxy(
      createRequest({
        cookies: {
          [cookieName]: "en",
        },
        headers: {
          "accept-language": "en",
          referer: "http://localhost/zh/dashboard",
        },
        url: "http://localhost/dashboard/content/1",
      }),
    );

    expect(response.headers.get("location")).toBe(
      "http://localhost/zh/dashboard/content/1",
    );
    expect(response.headers.get("set-cookie")).toContain(`${cookieName}=zh`);
  });
});

import { afterEach, beforeEach, describe, expect, it } from "vitest";
import nextConfig from "./next.config";

const originalBackendApiBaseUrl = process.env.BACKEND_API_BASE_URL;

describe("next config", () => {
  beforeEach(() => {
    process.env.BACKEND_API_BASE_URL = "http://backend.example:8080/";
  });

  afterEach(() => {
    process.env.BACKEND_API_BASE_URL = originalBackendApiBaseUrl;
  });

  it("rewrites browser stream requests directly to the backend", async () => {
    const rewrites =
      typeof nextConfig.rewrites === "function"
        ? await nextConfig.rewrites()
        : undefined;

    expect(rewrites).toEqual({
      beforeFiles: [
        {
          destination: "http://backend.example:8080/api/browser-stream/:path*",
          source: "/api/browser-stream/:path*",
        },
        {
          destination:
            "http://backend.example:8080/api/user/dashboard/browser-sessions/:path*",
          source: "/api/user/dashboard/browser-sessions/:path*",
        },
      ],
    });
  });
});

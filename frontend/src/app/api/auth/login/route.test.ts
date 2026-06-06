import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Mock } from "vitest";
import type { NextRequest } from "next/server";
import { POST } from "./route";

const originalBackendApiBaseUrl = process.env.BACKEND_API_BASE_URL;

type TestRequest = NextRequest & {
  arrayBuffer: Mock<() => Promise<ArrayBuffer>>;
};

function createRequest() {
  const body = new TextEncoder().encode(
    JSON.stringify({ username: "test_user", password: "Password1234" }),
  ).buffer;

  return {
    arrayBuffer: vi.fn(async () => body),
    headers: new Headers({
      "content-type": "application/json",
      host: "localhost:3000",
    }),
    method: "POST",
    nextUrl: new URL("http://localhost:3000/api/auth/login"),
  } as unknown as TestRequest;
}

describe("auth login route", () => {
  beforeEach(() => {
    process.env.BACKEND_API_BASE_URL = "http://backend:8080";
  });

  afterEach(() => {
    process.env.BACKEND_API_BASE_URL = originalBackendApiBaseUrl;
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("converts backend login tokens into HttpOnly session cookies", async () => {
    const fetchMock = vi.fn<typeof fetch>(
      async () =>
        new Response(JSON.stringify({ token: "jwt-token", username: "u" }), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const response = await POST(createRequest());
    const body = await response.json();
    const setCookieHeader = response.headers.get("set-cookie");

    expect(body).toEqual({ authenticated: true, username: "u" });
    expect(JSON.stringify(body)).not.toContain("jwt-token");
    expect(setCookieHeader).toContain("sevenoxcloud.auth_token=jwt-token");
    expect(setCookieHeader).toContain("HttpOnly");
    expect(setCookieHeader).toContain("SameSite=lax");

    const [targetUrl, init] = fetchMock.mock.calls[0];
    expect(String(targetUrl)).toBe("http://backend:8080/api/auth/login");
    expect(init?.method).toBe("POST");
  });
});

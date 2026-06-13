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
    JSON.stringify({
      code: "123456",
      email: "new@example.com",
      password: "Password1234",
      username: "new_user",
    }),
  ).buffer;

  return {
    arrayBuffer: vi.fn(async () => body),
    headers: new Headers({
      "content-type": "application/json",
      host: "localhost:3000",
    }),
    method: "POST",
    nextUrl: new URL("http://localhost:3000/api/auth/register"),
  } as unknown as TestRequest;
}

describe("auth register route", () => {
  beforeEach(() => {
    process.env.BACKEND_API_BASE_URL = "http://backend:8080";
  });

  afterEach(() => {
    process.env.BACKEND_API_BASE_URL = originalBackendApiBaseUrl;
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("converts backend registration tokens into HttpOnly session cookies", async () => {
    const fetchMock = vi.fn<typeof fetch>(
      async () =>
        new Response(JSON.stringify({ token: "new-jwt-token" }), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const response = await POST(createRequest());
    const body = await response.json();
    const setCookieHeader = response.headers.get("set-cookie");

    expect(body).toEqual({ authenticated: true });
    expect(JSON.stringify(body)).not.toContain("new-jwt-token");
    expect(response.headers.get("cache-control")).toBe("no-store, private");
    expect(setCookieHeader).toContain("sevenoxcloud.auth_token=new-jwt-token");
    expect(setCookieHeader).toContain("HttpOnly");

    const [targetUrl, init] = fetchMock.mock.calls[0];
    expect(String(targetUrl)).toBe("http://backend:8080/api/auth/register");
    expect(init?.method).toBe("POST");
  });
});

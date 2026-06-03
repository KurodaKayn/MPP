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
    JSON.stringify({ email: "user@example.com", scene: "register" }),
  ).buffer;

  return {
    arrayBuffer: vi.fn(async () => body),
    cookies: {
      get: () => undefined,
    },
    headers: new Headers({
      "content-type": "application/json",
      host: "localhost:3000",
    }),
    method: "POST",
    nextUrl: new URL("http://localhost:3000/api/auth/send-code"),
  } as unknown as TestRequest;
}

describe("auth api proxy route", () => {
  beforeEach(() => {
    process.env.BACKEND_API_BASE_URL = "http://backend:8080";
  });

  afterEach(() => {
    process.env.BACKEND_API_BASE_URL = originalBackendApiBaseUrl;
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("forwards auth subpaths through the backend auth prefix", async () => {
    const fetchMock = vi.fn<typeof fetch>(
      async () => new Response(null, { status: 204 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const response = await POST(createRequest(), {
      params: Promise.resolve({ path: ["send-code"] }),
    });

    expect(response.status).toBe(204);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [targetUrl, init] = fetchMock.mock.calls[0];
    expect(String(targetUrl)).toBe("http://backend:8080/api/auth/send-code");
    expect(init?.method).toBe("POST");
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Mock } from "vitest";
import type { NextRequest } from "next/server";
import { GET, POST } from "./route";

const originalBackendApiBaseUrl = process.env.BACKEND_API_BASE_URL;

type TestRequest = NextRequest & {
  arrayBuffer: Mock<() => Promise<ArrayBuffer>>;
};

function createRequest({
  body,
  cookies = {},
  headers = {},
  method = "GET",
  url = "http://localhost/api/dashboard/stats",
}: {
  body?: ArrayBuffer;
  cookies?: Record<string, string>;
  headers?: Record<string, string>;
  method?: string;
  url?: string;
} = {}) {
  const cookieStore = new Map(Object.entries(cookies));

  return {
    arrayBuffer: vi.fn(async () => body ?? new ArrayBuffer(0)),
    cookies: {
      get: (name: string) => {
        const value = cookieStore.get(name);
        return value ? { name, value } : undefined;
      },
    },
    headers: new Headers(headers),
    method,
    nextUrl: new URL(url),
  } as unknown as TestRequest;
}

function createContext(path: string[]) {
  return {
    params: Promise.resolve({ path }),
  };
}

describe("api proxy route", () => {
  beforeEach(() => {
    process.env.BACKEND_API_BASE_URL = "https://backend.example/root/";
  });

  afterEach(() => {
    process.env.BACKEND_API_BASE_URL = originalBackendApiBaseUrl;
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("forwards encoded path, query, sanitized headers, and cookie auth", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => {
      return new Response("proxied", {
        headers: {
          connection: "close",
          "transfer-encoding": "chunked",
          "x-backend": "ok",
        },
        status: 201,
        statusText: "Created",
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const request = createRequest({
      cookies: { auth_token: "raw-token" },
      headers: {
        connection: "keep-alive",
        host: "frontend.local",
        "x-client": "web",
      },
      method: "GET",
      url: "http://localhost/api/dashboard/stats?search=a%20b",
    });

    const response = await GET(
      request,
      createContext(["dashboard", "项目 stats"]),
    );

    expect(fetchMock).toHaveBeenCalledOnce();
    const [targetUrl, init] = fetchMock.mock.calls[0];
    expect(String(targetUrl)).toBe(
      "https://backend.example/api/dashboard/%E9%A1%B9%E7%9B%AE%20stats?search=a%20b",
    );
    expect(init?.method).toBe("GET");
    expect(init?.body).toBeUndefined();
    expect(request.arrayBuffer).not.toHaveBeenCalled();
    expect(init?.cache).toBe("no-store");
    expect(init?.redirect).toBe("manual");

    const forwardedHeaders = init?.headers as Headers;
    expect(forwardedHeaders.get("authorization")).toBe("Bearer raw-token");
    expect(forwardedHeaders.get("x-client")).toBe("web");
    expect(forwardedHeaders.has("connection")).toBe(false);
    expect(forwardedHeaders.has("host")).toBe(false);

    expect(response.status).toBe(201);
    expect(response.headers.get("x-backend")).toBe("ok");
    expect(response.headers.has("connection")).toBe(false);
    expect(response.headers.has("transfer-encoding")).toBe(false);
  });

  it("preserves explicit authorization and forwards a body for write methods", async () => {
    const fetchMock = vi.fn<typeof fetch>(
      async () => new Response(null, { status: 204 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const body = new TextEncoder().encode("payload").buffer;
    const request = createRequest({
      body,
      cookies: { access_token: "cookie-token" },
      headers: { authorization: "Bearer header-token" },
      method: "POST",
      url: "http://localhost/api/dashboard/projects",
    });

    const response = await POST(
      request,
      createContext(["dashboard", "projects"]),
    );

    expect(response.status).toBe(204);
    expect(request.arrayBuffer).toHaveBeenCalledOnce();

    const [, init] = fetchMock.mock.calls[0];
    expect(init).toBeDefined();
    expect(init?.method).toBe("POST");
    expect(init?.body).toBe(body);
    const forwardedHeaders = init!.headers as Headers;
    expect(forwardedHeaders.get("authorization")).toBe("Bearer header-token");
  });
});

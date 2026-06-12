import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
  type Mock,
} from "vitest";
import { cookies } from "next/headers";
import { DELETE, GET, POST } from "./route";

vi.mock("next/headers", () => ({
  cookies: vi.fn(),
}));

const cookiesMock = cookies as unknown as Mock;
const originalAppEnv = process.env.APP_ENV;
const originalBackendApiBaseUrl = process.env.BACKEND_API_BASE_URL;
const originalEnableMockLogin = process.env.ENABLE_MOCK_LOGIN;

function setCookieStore(values: Record<string, string>) {
  cookiesMock.mockResolvedValue({
    get: (name: string) => {
      const value = values[name];
      return value ? { name, value } : undefined;
    },
  });
}

function createPostRequest(token: string) {
  return {
    headers: new Headers(),
    json: vi.fn(async () => ({ token })),
    nextUrl: new URL("http://localhost:3000/api/auth/session"),
  } as never;
}

describe("auth session route", () => {
  beforeEach(() => {
    process.env.ENABLE_MOCK_LOGIN = "false";
  });

  afterEach(() => {
    process.env.APP_ENV = originalAppEnv;
    process.env.BACKEND_API_BASE_URL = originalBackendApiBaseUrl;
    process.env.ENABLE_MOCK_LOGIN = originalEnableMockLogin;
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("verifies cookie-backed sessions without exposing the token", async () => {
    process.env.BACKEND_API_BASE_URL = "https://backend.example/root/";
    const fetchMock = vi.fn<typeof fetch>(
      async () => new Response(JSON.stringify({ projects: 0 })),
    );
    vi.stubGlobal("fetch", fetchMock);
    setCookieStore({ auth_token: "cookie-token" });

    const response = await GET();
    const body = await response.json();

    expect(body).toEqual({
      authenticated: true,
      loginMethods: {
        mock: false,
        token: true,
      },
    });

    expect(fetchMock).toHaveBeenCalledOnce();
    const [targetUrl, init] = fetchMock.mock.calls[0];
    expect(String(targetUrl)).toBe(
      "https://backend.example/api/user/dashboard/stats",
    );
    expect(init?.cache).toBe("no-store");
    const headers = init?.headers as HeadersInit;
    expect(new Headers(headers).get("authorization")).toBe(
      "Bearer cookie-token",
    );
  });

  it("clears expired cookie-backed sessions", async () => {
    const fetchMock = vi.fn<typeof fetch>(
      async () => new Response(null, { status: 401 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    setCookieStore({ auth_token: "expired-token" });

    const response = await GET();
    const body = await response.json();
    const setCookieHeader = response.headers.get("set-cookie");

    expect(body.authenticated).toBe(false);
    expect(setCookieHeader).toContain("sevenoxcloud.auth_token=");
    expect(setCookieHeader).toContain("Max-Age=0");
    expect(setCookieHeader).toContain("HttpOnly");
  });

  it("sets an HttpOnly cookie when posting a valid access token", async () => {
    process.env.BACKEND_API_BASE_URL = "https://backend.example/root/";
    const fetchMock = vi.fn<typeof fetch>(
      async () => new Response(JSON.stringify({ projects: 0 })),
    );
    vi.stubGlobal("fetch", fetchMock);

    const response = await POST(createPostRequest("session-token"));
    const body = await response.json();
    const setCookieHeader = response.headers.get("set-cookie");

    expect(body).toEqual({
      authenticated: true,
      loginMethods: {
        mock: false,
        token: true,
      },
    });
    expect(setCookieHeader).toContain("sevenoxcloud.auth_token=session-token");
    expect(setCookieHeader).toContain("HttpOnly");
    expect(setCookieHeader).toContain("SameSite=lax");
    expect(JSON.stringify(body)).not.toContain("session-token");

    const [, init] = fetchMock.mock.calls[0];
    expect(new Headers(init?.headers).get("authorization")).toBe(
      "Bearer session-token",
    );
  });

  it("rejects invalid access tokens and clears stale cookies", async () => {
    const fetchMock = vi.fn<typeof fetch>(
      async () => new Response(null, { status: 401 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const response = await POST(createPostRequest("expired-token"));
    const body = await response.json();
    const setCookieHeader = response.headers.get("set-cookie");

    expect(response.status).toBe(401);
    expect(body.error.code).toBe("invalid_token");
    expect(setCookieHeader).toContain("sevenoxcloud.auth_token=");
    expect(setCookieHeader).toContain("Max-Age=0");
    expect(setCookieHeader).toContain("HttpOnly");
  });

  it("reports mock login only when explicitly enabled for local development", async () => {
    process.env.APP_ENV = "development";
    process.env.ENABLE_MOCK_LOGIN = "true";
    const fetchMock = vi.fn<typeof fetch>();
    vi.stubGlobal("fetch", fetchMock);
    setCookieStore({});

    const response = await GET();
    const body = await response.json();

    expect(body.loginMethods.mock).toBe(true);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("expires supported auth cookies on delete", () => {
    const response = DELETE();
    const setCookieHeader = response.headers.get("set-cookie");

    expect(setCookieHeader).toContain("sevenoxcloud.auth_token=");
    expect(setCookieHeader).toContain("Max-Age=0");
    expect(setCookieHeader).toContain("HttpOnly");
  });
});

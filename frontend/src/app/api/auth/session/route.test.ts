import { afterEach, describe, expect, it, vi, type Mock } from "vitest";
import { cookies } from "next/headers";
import { DELETE, GET } from "./route";

vi.mock("next/headers", () => ({
  cookies: vi.fn(),
}));

const cookiesMock = cookies as unknown as Mock;
const originalAppEnv = process.env.APP_ENV;
const originalEnableMockLogin = process.env.ENABLE_MOCK_LOGIN;

function setCookieStore(values: Record<string, string>) {
  cookiesMock.mockResolvedValue({
    get: (name: string) => {
      const value = values[name];
      return value ? { name, value } : undefined;
    },
  });
}

describe("auth session route", () => {
  afterEach(() => {
    process.env.APP_ENV = originalAppEnv;
    process.env.ENABLE_MOCK_LOGIN = originalEnableMockLogin;
    vi.restoreAllMocks();
  });

  it("reports cookie-backed sessions without exposing the token", async () => {
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
  });

  it("reports mock login only when explicitly enabled for local development", async () => {
    process.env.APP_ENV = "development";
    process.env.ENABLE_MOCK_LOGIN = "true";
    setCookieStore({});

    const response = await GET();
    const body = await response.json();

    expect(body.loginMethods.mock).toBe(true);
  });

  it("expires supported auth cookies on delete", () => {
    const response = DELETE();
    const setCookieHeader = response.headers.get("set-cookie");

    expect(setCookieHeader).toContain("sevenoxcloud.auth_token=");
    expect(setCookieHeader).toContain("Max-Age=0");
  });
});

// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { authTokenNames, primaryAuthTokenName } from "./tokens";
import {
  getCurrentAuthState,
  loginWithAccessToken,
  loginWithUsername,
} from "./client";

function jsonResponse(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    ...init,
  });
}

describe("auth client", () => {
  beforeEach(() => {
    window.localStorage.clear();
    window.sessionStorage.clear();
    for (const name of authTokenNames) {
      document.cookie = `${name}=; path=/; max-age=0`;
    }
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("hydrates a cookie-only session from the auth status endpoint", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse({
        authenticated: true,
        loginMethods: { mock: false, token: true },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const authState = await getCurrentAuthState();

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/session",
      expect.objectContaining({
        cache: "no-store",
        credentials: "same-origin",
      }),
    );
    expect(authState.session).toEqual({ token: null, username: "Creator" });
  });

  it("does not call the local mock login endpoint when it is unavailable", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse({
        authenticated: false,
        loginMethods: { mock: false, token: true },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(loginWithUsername("test_user")).rejects.toThrow(
      "开发账号登录仅在本地开发环境可用",
    );

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0][0]).toBe("/api/auth/session");
  });

  it("verifies and stores an access token login", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse({
        total_failed_publications: 0,
        total_projects: 0,
        total_published_publications: 0,
        total_users: 1,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(loginWithAccessToken("raw-token")).resolves.toEqual({
      token: "raw-token",
      username: "Creator",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/stats",
      expect.objectContaining({
        cache: "no-store",
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
    const [, init] = fetchMock.mock.calls[0];
    const headers = init?.headers as Headers;
    expect(headers.get("Authorization")).toBe("Bearer raw-token");
    expect(window.localStorage.getItem(primaryAuthTokenName)).toBe("raw-token");
  });
});

// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { authTokenNames, primaryAuthTokenName } from "./tokens";
import {
  getCurrentAuthState,
  loginWithAccessToken,
  loginWithUsername,
  registerWithCredentials,
  resetPassword,
  sendAuthCode,
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

    await expect(
      loginWithUsername("test_user", "Password1234"),
    ).rejects.toThrow(
      "Dev account login is only available in local development",
    );

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0][0]).toBe("/api/auth/session");
  });

  it("creates a server access-token session without storing the token in browser storage", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse({
        authenticated: true,
        loginMethods: { mock: false, token: true },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(loginWithAccessToken("raw-token")).resolves.toEqual({
      token: null,
      username: "Creator",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/session",
      expect.objectContaining({
        body: JSON.stringify({ token: "raw-token" }),
        cache: "no-store",
        credentials: "same-origin",
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }),
    );
    expect(window.localStorage.getItem(primaryAuthTokenName)).toBeNull();
    expect(window.sessionStorage.getItem(primaryAuthTokenName)).toBeNull();
    expect(document.cookie).not.toContain(primaryAuthTokenName);
  });

  it("logs in with username and password without exposing the returned token", async () => {
    const fetchMock = vi.fn<typeof fetch>(async (input) => {
      if (input === "/api/auth/session") {
        return jsonResponse({
          authenticated: false,
          loginMethods: { mock: true, token: true },
        });
      }

      return jsonResponse({ authenticated: true });
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      loginWithUsername("test_user", "Password1234"),
    ).resolves.toEqual({
      token: null,
      username: "test_user",
    });

    expect(fetchMock).toHaveBeenLastCalledWith(
      "/api/auth/login",
      expect.objectContaining({
        body: JSON.stringify({
          username: "test_user",
          password: "Password1234",
        }),
        method: "POST",
      }),
    );
    expect(window.localStorage.getItem(primaryAuthTokenName)).toBeNull();
    expect(window.sessionStorage.getItem(primaryAuthTokenName)).toBeNull();
    expect(document.cookie).not.toContain(primaryAuthTokenName);
  });

  it("registers with credentials without exposing the returned token", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse({ authenticated: true }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      registerWithCredentials(
        "new_user",
        "new@example.com",
        "123456",
        "Password1234",
      ),
    ).resolves.toEqual({
      token: null,
      username: "new_user",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/register",
      expect.objectContaining({
        body: JSON.stringify({
          code: "123456",
          email: "new@example.com",
          password: "Password1234",
          username: "new_user",
        }),
        method: "POST",
      }),
    );
    expect(window.localStorage.getItem(primaryAuthTokenName)).toBeNull();
    expect(window.sessionStorage.getItem(primaryAuthTokenName)).toBeNull();
    expect(document.cookie).not.toContain(primaryAuthTokenName);
  });

  it("sends an auth verification code", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse({}));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      sendAuthCode("new@example.com", "register"),
    ).resolves.toBeUndefined();

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/send-code",
      expect.objectContaining({
        body: JSON.stringify({
          email: "new@example.com",
          scene: "register",
        }),
        method: "POST",
      }),
    );
  });

  it("resets a password with an email verification code", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse({}));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      resetPassword("user@example.com", "654321", "NewPassword1234"),
    ).resolves.toBeUndefined();

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/reset-password",
      expect.objectContaining({
        body: JSON.stringify({
          email: "user@example.com",
          code: "654321",
          password: "NewPassword1234",
        }),
        method: "POST",
      }),
    );
  });
});

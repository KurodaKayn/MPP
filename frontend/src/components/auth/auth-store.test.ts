// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { authTokenNames, primaryAuthTokenName } from "@/lib/auth/tokens";
import { useAuthStore } from "./auth-store";

function jsonResponse(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    ...init,
  });
}

describe("useAuthStore", () => {
  beforeEach(() => {
    useAuthStore.getState().reset();
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

  it("refreshes auth state from the current browser session", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async () =>
        jsonResponse({
          authenticated: true,
          loginMethods: { mock: false, token: true },
          username: "CookieUser",
        }),
      ),
    );

    await useAuthStore.getState().refresh();

    expect(useAuthStore.getState()).toMatchObject({
      initialized: true,
      loginMethods: { mock: false, token: true },
      session: { token: null, username: "CookieUser" },
    });
  });

  it("stores an access token login state without keeping the token in web storage", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async () =>
        jsonResponse({
          authenticated: true,
          loginMethods: { mock: false, token: true },
        }),
      ),
    );

    await expect(
      useAuthStore.getState().loginWithToken("raw-token"),
    ).resolves.toEqual({
      token: null,
      username: "Creator",
    });

    expect(useAuthStore.getState().session).toEqual({
      token: null,
      username: "Creator",
    });
    expect(window.localStorage.getItem(primaryAuthTokenName)).toBeNull();
    expect(window.sessionStorage.getItem(primaryAuthTokenName)).toBeNull();
  });

  it("clears client and server auth state on logout", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse({}));
    vi.stubGlobal("fetch", fetchMock);
    window.localStorage.setItem(primaryAuthTokenName, "raw-token");
    useAuthStore.setState({
      initialized: true,
      session: { token: null, username: "Creator" },
    });

    await useAuthStore.getState().logout();

    expect(useAuthStore.getState().session).toBeNull();
    expect(window.localStorage.getItem(primaryAuthTokenName)).toBeNull();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/session",
      expect.objectContaining({ method: "DELETE" }),
    );
  });
});

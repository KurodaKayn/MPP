// @vitest-environment jsdom

import { describe, expect, it, vi } from "vitest";
import {
  cancelBrowserSession,
  completeBrowserSession,
  getBrowserSession,
  getDouyinAccount,
  getWechatAccount,
  getXAccount,
  saveWechatAccount,
  saveXAccount,
  startBrowserSession,
  testWechatConnection,
  testXConnection,
} from "./api";
import {
  jsonResponse,
  setupDashboardApiTest,
} from "./api-test-utils";

describe("dashboard account api", () => {
  setupDashboardApiTest();

  it("does not cache repeated browser session polls", async () => {
    const session = {
      session_id: "session-1",
      status: "running",
      stream_url: "/stream",
    };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(session))
      .mockResolvedValueOnce(jsonResponse(session));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getBrowserSession("session-1")).resolves.toEqual(session);
    await expect(getBrowserSession("session-1")).resolves.toEqual(session);

    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("fetches and updates the WeChat account settings", async () => {
    const account = {
      account_auth: {
        message: "WeChat account verification needs manual confirmation",
        status: "unknown",
        title: "Automatic confirmation unavailable",
      },
      app_id: "wx-app",
      has_app_secret: true,
      ip_whitelist: {
        message: "Waiting for verification",
        status: "unknown",
        title: "Waiting for verification",
      },
      platform: "wechat",
      status: "untested",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(account));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getWechatAccount()).resolves.toEqual(account);
    await expect(
      saveWechatAccount({ app_id: "wx-app", app_secret: "wx-secret" }),
    ).resolves.toEqual(account);

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/user/dashboard/settings/wechat/account",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/user/dashboard/settings/wechat/account",
      expect.objectContaining({
        body: JSON.stringify({
          app_id: "wx-app",
          app_secret: "wx-secret",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "PUT",
      }),
    );
  });

  it("posts WeChat connection test credentials", async () => {
    const result = {
      account_auth: {
        message: "Connection success does not guarantee publish permission",
        status: "warning",
        title: "Verify auth and publish permissions",
      },
      connected: true,
      ip_whitelist: {
        message: "The WeChat API accepted the current server request",
        status: "passed",
        title: "IP allowlist verified",
      },
      message: "Connected",
      status: "connected",
      tested_at: "2026-05-29T12:00:00Z",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(result));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      testWechatConnection({ app_id: "wx-app", app_secret: "wx-secret" }),
    ).resolves.toEqual(result);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/settings/wechat/test",
      expect.objectContaining({
        body: JSON.stringify({
          app_id: "wx-app",
          app_secret: "wx-secret",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "POST",
      }),
    );
  });

  it("fetches and updates the X account settings", async () => {
    const account = {
      account_auth: {
        message: "Account credentials verified",
        status: "passed",
        title: "Account credentials verified",
      },
      api_key: "x-api-key",
      has_access_token: true,
      has_access_token_secret: true,
      has_api_secret: true,
      platform: "x",
      publish_access: {
        message:
          "Before publishing, confirm the X App has Read and write user permission.",
        status: "unknown",
        title: "Waiting for verification",
      },
      status: "untested",
      username: "creator",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(account));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getXAccount()).resolves.toEqual(account);
    await expect(
      saveXAccount({
        access_token: "x-access-token",
        access_token_secret: "x-access-secret",
        api_key: "x-api-key",
        api_secret: "x-api-secret",
        username: "creator",
      }),
    ).resolves.toEqual(account);

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/user/dashboard/settings/x/account",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/user/dashboard/settings/x/account",
      expect.objectContaining({
        body: JSON.stringify({
          access_token: "x-access-token",
          access_token_secret: "x-access-secret",
          api_key: "x-api-key",
          api_secret: "x-api-secret",
          username: "creator",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "PUT",
      }),
    );
  });

  it("posts X connection test credentials", async () => {
    const result = {
      account_auth: {
        message: "Connected as @creator.",
        status: "passed",
        title: "Account credentials verified",
      },
      connected: true,
      message: "Connected",
      name: "Creator",
      publish_access: {
        message:
          "The test verifies account identity; actual publishing also requires X App Read and write permission.",
        status: "warning",
        title: "Confirm write permission",
      },
      status: "connected",
      tested_at: "2026-05-29T12:00:00Z",
      user_id: "123",
      username: "creator",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(result));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      testXConnection({
        access_token: "x-access-token",
        access_token_secret: "x-access-secret",
        api_key: "x-api-key",
        api_secret: "x-api-secret",
      }),
    ).resolves.toEqual(result);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/settings/x/test",
      expect.objectContaining({
        body: JSON.stringify({
          access_token: "x-access-token",
          access_token_secret: "x-access-secret",
          api_key: "x-api-key",
          api_secret: "x-api-secret",
        }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "POST",
      }),
    );
  });

  it("fetches Douyin account status", async () => {
    const account = {
      platform: "douyin",
      status: "connected",
      updated_at: "2026-05-31T12:00:00Z",
      username: "creator",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(account));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getDouyinAccount()).resolves.toEqual(account);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/dashboard/settings/douyin/account",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
  });

  it("controls remote browser sessions", async () => {
    const start = {
      expires_at: "2026-05-31T12:15:00Z",
      session_id: "session-1",
      status: "ready",
      stream_token_expires_at: "2026-05-31T12:05:00Z",
      stream_url:
        "/api/user/dashboard/browser-sessions/session-1/stream?token=t",
    };
    const session = {
      ...start,
      platform: "douyin",
    };
    const complete = {
      account: { avatar_url: "", username: "creator" },
      message: "Connected",
      platform: "douyin",
      session_id: "session-1",
      status: "connected",
    };
    const cancel = {
      session_id: "session-1",
      status: "expired",
    };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(start))
      .mockResolvedValueOnce(jsonResponse(session))
      .mockResolvedValueOnce(jsonResponse(complete))
      .mockResolvedValueOnce(jsonResponse(cancel));
    vi.stubGlobal("fetch", fetchMock);

    await expect(startBrowserSession("douyin")).resolves.toEqual(start);
    await expect(getBrowserSession("session-1")).resolves.toEqual(session);
    await expect(completeBrowserSession("session-1")).resolves.toEqual(
      complete,
    );
    await expect(cancelBrowserSession("session-1")).resolves.toEqual(cancel);

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/user/dashboard/settings/platforms/douyin/browser-session",
      expect.objectContaining({ method: "POST" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/user/dashboard/browser-sessions/session-1",
      expect.objectContaining({ credentials: "same-origin" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "/api/user/dashboard/browser-sessions/session-1/complete",
      expect.objectContaining({ method: "POST" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      4,
      "/api/user/dashboard/browser-sessions/session-1",
      expect.objectContaining({ method: "DELETE" }),
    );
  });
});

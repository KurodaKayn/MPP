import { describe, expect, it, vi } from "vitest";
import {
  clearExtensionAuthSession,
  clearStoredExtensionAuthTokens,
  getExtensionAuthToken,
  getWebAuthTokenFromStorage,
  getStoredExtensionAuthToken,
  persistExtensionAuthToken,
} from "./auth";

describe("getStoredExtensionAuthToken", () => {
  it("returns the first stored MPP auth token", async () => {
    const storage = {
      get: vi.fn().mockResolvedValue({
        "sevenoxcloud.auth_token": "",
        auth_token: "secondary-token",
        access_token: "access-token",
      }),
    };

    await expect(getStoredExtensionAuthToken(storage)).resolves.toBe(
      "secondary-token",
    );
  });

  it("normalizes bearer-prefixed tokens before backend client usage", async () => {
    const storage = {
      get: vi.fn().mockResolvedValue({
        "sevenoxcloud.auth_token": "Bearer raw-token",
      }),
    };

    await expect(getStoredExtensionAuthToken(storage)).resolves.toBe(
      "raw-token",
    );
  });

  it("returns null when extension storage has no web auth token", async () => {
    const storage = {
      get: vi.fn().mockResolvedValue({}),
    };

    await expect(getStoredExtensionAuthToken(storage)).resolves.toBeNull();
  });
});

describe("getExtensionAuthToken", () => {
  it("reads the MPP web login token from HttpOnly cookies", async () => {
    const cookies = {
      get: vi.fn().mockResolvedValue({ value: "Bearer cookie-token" }),
    };
    const storage = {
      get: vi.fn().mockResolvedValue({}),
    };

    await expect(
      getExtensionAuthToken({
        cookies,
        storage,
        webBaseUrl: "http://localhost:3000",
      }),
    ).resolves.toBe("cookie-token");

    expect(cookies.get).toHaveBeenCalledWith({
      name: "sevenoxcloud.auth_token",
      url: "http://localhost:3000",
    });
    expect(storage.get).not.toHaveBeenCalled();
  });

  it("falls back to extension storage when web login cookies are unavailable", async () => {
    const cookies = {
      get: vi.fn().mockResolvedValue(null),
    };
    const storage = {
      get: vi.fn().mockResolvedValue({
        "sevenoxcloud.auth_token": "stored-token",
      }),
    };

    await expect(
      getExtensionAuthToken({
        cookies,
        storage,
        webBaseUrl: "http://localhost:3000",
      }),
    ).resolves.toBe("stored-token");
  });
});

describe("persistExtensionAuthToken", () => {
  it("stores normalized web auth tokens under the canonical extension key", async () => {
    const storage = {
      get: vi.fn(),
      set: vi.fn().mockResolvedValue(undefined),
    };

    await persistExtensionAuthToken("Bearer web-token", storage);

    expect(storage.set).toHaveBeenCalledWith({
      "sevenoxcloud.auth_token": "web-token",
    });
  });

  it("does not store empty web auth tokens", async () => {
    const storage = {
      get: vi.fn(),
      set: vi.fn().mockResolvedValue(undefined),
    };

    await persistExtensionAuthToken("   ", storage);

    expect(storage.set).not.toHaveBeenCalled();
  });
});

describe("clearStoredExtensionAuthTokens", () => {
  it("removes all extension auth token keys from local storage", async () => {
    const storage = {
      remove: vi.fn().mockResolvedValue(undefined),
    };

    await clearStoredExtensionAuthTokens(storage);

    expect(storage.remove).toHaveBeenCalledWith([
      "sevenoxcloud.auth_token",
      "auth_token",
      "access_token",
      "mpp.web_auth_token",
    ]);
  });
});

describe("clearExtensionAuthSession", () => {
  it("removes extension tokens and MPP web auth cookies", async () => {
    const storage = {
      remove: vi.fn().mockResolvedValue(undefined),
    };
    const cookies = {
      remove: vi.fn().mockResolvedValue(null),
    };

    await clearExtensionAuthSession({
      cookies,
      storage,
      webBaseUrl: "http://localhost:3000/zh/login",
    });

    expect(storage.remove).toHaveBeenCalledWith([
      "sevenoxcloud.auth_token",
      "auth_token",
      "access_token",
      "mpp.web_auth_token",
    ]);
    expect(cookies.remove).toHaveBeenCalledWith({
      name: "sevenoxcloud.auth_token",
      url: "http://localhost:3000",
    });
    expect(cookies.remove).toHaveBeenCalledWith({
      name: "auth_token",
      url: "http://localhost:3000",
    });
    expect(cookies.remove).toHaveBeenCalledWith({
      name: "access_token",
      url: "http://localhost:3000",
    });
  });
});

describe("getWebAuthTokenFromStorage", () => {
  it("reads the first available MPP web token from page localStorage", () => {
    const storage = {
      getItem: vi.fn((key: string) =>
        key === "auth_token" ? "Bearer page-token" : null,
      ),
    };

    expect(getWebAuthTokenFromStorage(storage)).toBe("page-token");
  });

  it("returns null when page localStorage has no MPP web token", () => {
    const storage = {
      getItem: vi.fn(() => null),
    };

    expect(getWebAuthTokenFromStorage(storage)).toBeNull();
  });
});

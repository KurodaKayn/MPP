import { describe, expect, it, vi } from "vitest";
import {
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

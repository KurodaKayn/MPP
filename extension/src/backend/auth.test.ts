import { describe, expect, it, vi } from "vitest";
import { getStoredExtensionAuthToken } from "./auth";

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

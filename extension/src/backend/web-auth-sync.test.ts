import { afterEach, describe, expect, it, vi } from "vitest";
import { startWebAuthTokenSync } from "./web-auth-sync";

describe("startWebAuthTokenSync", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("persists a web token that appears after the page has loaded", async () => {
    vi.useFakeTimers();

    let token: string | null = null;
    const persistToken = vi.fn().mockResolvedValue(undefined);

    const stop = startWebAuthTokenSync({
      intervalMs: 1000,
      maxAttempts: 3,
      persistToken,
      readToken: () => token,
    });

    expect(persistToken).not.toHaveBeenCalled();

    token = "late-web-token";
    await vi.advanceTimersByTimeAsync(1000);

    expect(persistToken).toHaveBeenCalledWith("late-web-token");

    stop();
  });

  it("does not persist the same web token repeatedly while polling", async () => {
    vi.useFakeTimers();

    const persistToken = vi.fn().mockResolvedValue(undefined);

    const stop = startWebAuthTokenSync({
      intervalMs: 1000,
      maxAttempts: 3,
      persistToken,
      readToken: () => "stable-web-token",
    });

    await vi.advanceTimersByTimeAsync(3000);

    expect(persistToken).toHaveBeenCalledTimes(1);

    stop();
  });
});

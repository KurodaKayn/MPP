// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAuthPageController } from "./use-auth-page-controller";

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

const mocks = vi.hoisted(() => ({
  cancelBrowserSession: vi.fn(),
  completeBrowserSession: vi.fn(),
  getBrowserSession: vi.fn(),
  getDouyinAccount: vi.fn(),
  getWechatAccount: vi.fn(),
  getXAccount: vi.fn(),
  getZhihuAccount: vi.fn(),
  replace: vi.fn(),
  saveWechatAccount: vi.fn(),
  saveXAccount: vi.fn(),
  startBrowserSession: vi.fn(),
  testWechatConnection: vi.fn(),
  testXConnection: vi.fn(),
  toastError: vi.fn(),
  toastSuccess: vi.fn(),
}));

vi.mock("@/lib/dashboard/api", () => ({
  cancelBrowserSession: mocks.cancelBrowserSession,
  completeBrowserSession: mocks.completeBrowserSession,
  getBrowserSession: mocks.getBrowserSession,
  getDouyinAccount: mocks.getDouyinAccount,
  getWechatAccount: mocks.getWechatAccount,
  getXAccount: mocks.getXAccount,
  getZhihuAccount: mocks.getZhihuAccount,
  saveWechatAccount: mocks.saveWechatAccount,
  saveXAccount: mocks.saveXAccount,
  startBrowserSession: mocks.startBrowserSession,
  testWechatConnection: mocks.testWechatConnection,
  testXConnection: mocks.testXConnection,
}));

vi.mock("@/lib/i18n/client", () => ({
  useAppLocale: () => "en",
  useTranslation: () => ({
    t: (key: string, options?: { platform?: string }) =>
      options?.platform ? `${key} ${options.platform}` : key,
  }),
}));

vi.mock("next/navigation", () => ({
  useParams: () => ({ locale: "en" }),
  useRouter: () => ({
    replace: mocks.replace,
  }),
  useSearchParams: () => new URLSearchParams(),
}));

vi.mock("sonner", () => ({
  toast: {
    error: mocks.toastError,
    success: mocks.toastSuccess,
  },
}));

type Controller = ReturnType<typeof useAuthPageController>;

const unknownRequirement = {
  message: "",
  status: "unknown" as const,
  title: "",
};

function renderController() {
  let controller: Controller | undefined;
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);

  function Harness() {
    controller = useAuthPageController();
    return null;
  }

  act(() => {
    root.render(<Harness />);
  });

  return {
    getController() {
      if (!controller) {
        throw new Error("Controller did not render.");
      }
      return controller;
    },
    unmount() {
      act(() => {
        root.unmount();
      });
      container.remove();
    },
  };
}

async function flushPromises() {
  await act(async () => {
    await Promise.resolve();
  });
}

describe("useAuthPageController", () => {
  beforeEach(() => {
    vi.stubEnv("NODE_ENV", "test");
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    vi.useFakeTimers();
    mocks.cancelBrowserSession.mockReset();
    mocks.completeBrowserSession.mockReset();
    mocks.getBrowserSession.mockReset();
    mocks.getDouyinAccount.mockReset();
    mocks.getWechatAccount.mockReset();
    mocks.getXAccount.mockReset();
    mocks.getZhihuAccount.mockReset();
    mocks.replace.mockReset();
    mocks.saveWechatAccount.mockReset();
    mocks.saveXAccount.mockReset();
    mocks.startBrowserSession.mockReset();
    mocks.testWechatConnection.mockReset();
    mocks.testXConnection.mockReset();
    mocks.toastError.mockReset();
    mocks.toastSuccess.mockReset();

    mocks.getWechatAccount.mockResolvedValue({
      account_auth: unknownRequirement,
      app_id: "",
      has_app_secret: false,
      ip_whitelist: unknownRequirement,
      platform: "wechat",
      status: "unconfigured",
    });
    mocks.getXAccount.mockResolvedValue({
      account_auth: unknownRequirement,
      has_access_token: false,
      has_access_token_secret: false,
      has_api_secret: false,
      platform: "x",
      publish_access: unknownRequirement,
      status: "unconfigured",
    });
    mocks.getDouyinAccount.mockResolvedValue({
      platform: "douyin",
      status: "unconfigured",
    });
    mocks.getZhihuAccount.mockResolvedValue({
      platform: "zhihu",
      status: "unconfigured",
    });
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.useRealTimers();
  });

  it("keeps an active remote browser iframe URL stable when polling rotates the token", async () => {
    const initialSession = {
      expires_at: "2026-06-01T12:15:00Z",
      session_id: "session-1",
      status: "ready",
      stream_token_expires_at: "2026-06-01T12:05:00Z",
      stream_url:
        "/api/user/dashboard/browser-sessions/session-1/stream/initial/vnc.html",
    };
    mocks.startBrowserSession.mockResolvedValue(initialSession);
    mocks.getBrowserSession.mockResolvedValue({
      ...initialSession,
      stream_token_expires_at: "2026-06-01T12:07:30Z",
      stream_url:
        "/api/user/dashboard/browser-sessions/session-1/stream/rotated/vnc.html",
    });
    const view = renderController();
    await flushPromises();

    act(() => {
      view.getController().douyin.onConnect();
    });
    await flushPromises();

    expect(view.getController().browserSession?.streamURL).toBe(
      initialSession.stream_url,
    );

    await act(async () => {
      vi.advanceTimersByTime(2500);
      await Promise.resolve();
    });

    expect(mocks.getBrowserSession).toHaveBeenCalledWith("session-1");
    expect(view.getController().browserSession?.streamURL).toBe(
      initialSession.stream_url,
    );

    view.unmount();
  });

  it("replaces the remote browser iframe URL after a stream error", async () => {
    const initialSession = {
      expires_at: "2026-06-01T12:15:00Z",
      session_id: "session-1",
      status: "ready",
      stream_token_expires_at: "2026-06-01T12:05:00Z",
      stream_url:
        "/api/user/dashboard/browser-sessions/session-1/stream/initial/vnc.html",
    };
    const rotatedSession = {
      ...initialSession,
      stream_token_expires_at: "2026-06-01T12:07:30Z",
      stream_url:
        "/api/user/dashboard/browser-sessions/session-1/stream/rotated/vnc.html",
    };
    mocks.startBrowserSession.mockResolvedValue(initialSession);
    mocks.getBrowserSession.mockResolvedValue(rotatedSession);
    const view = renderController();
    await flushPromises();

    act(() => {
      view.getController().douyin.onConnect();
    });
    await flushPromises();

    act(() => {
      view.getController().browserSession?.onStreamError?.();
    });

    await act(async () => {
      vi.advanceTimersByTime(2500);
      await Promise.resolve();
    });

    expect(view.getController().browserSession?.streamURL).toBe(
      rotatedSession.stream_url,
    );

    view.unmount();
  });
});

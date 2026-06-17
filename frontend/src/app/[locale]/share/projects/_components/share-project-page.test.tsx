// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AcceptProjectShareLinkResponse } from "@/lib/dashboard/api";

import { ShareProjectPage } from "./share-project-page";

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

const selectedWorkspaceStorageKey = "mpp.dashboard.selectedWorkspaceId";

type AuthStateMock = {
  initialized: boolean;
  session: { user: { id: string } } | null;
};

const mocks = vi.hoisted(() => {
  const replace = vi.fn();
  const translate = (key: string) => {
    const translations: Record<string, string> = {
      "shareProject.accepted": "Project access granted",
      "shareProject.badge": "Project invitation",
      "shareProject.continue": "Continue",
      "shareProject.description":
        "Accepting the invitation will add this project to your dashboard.",
      "shareProject.errorDescription": "This invitation could not be accepted.",
      "shareProject.retry": "Try again",
      "shareProject.retryLater": "Please try again later.",
      "shareProject.title": "Open shared project",
    };
    return translations[key] ?? key;
  };

  return {
    acceptProjectShareLink: vi.fn(),
    authState: {
      initialized: true,
      session: { user: { id: "user-1" } },
    } as AuthStateMock,
    pathname: "/en/share/projects",
    replace,
    router: {
      replace,
    },
    toastSuccess: vi.fn(),
    translate,
  };
});

vi.mock("@/components/auth/auth-provider", () => ({
  useAuth: () => mocks.authState,
}));

vi.mock("@/lib/dashboard/api", () => ({
  acceptProjectShareLink: mocks.acceptProjectShareLink,
}));

vi.mock("@/lib/i18n/client", () => ({
  useAppLocale: () => "en",
  useTranslation: () => ({ t: mocks.translate }),
}));

vi.mock("next/navigation", () => ({
  usePathname: () => mocks.pathname,
  useRouter: () => mocks.router,
}));

vi.mock("sonner", () => ({
  toast: {
    success: mocks.toastSuccess,
  },
}));

type Deferred<T> = {
  promise: Promise<T>;
  reject: (reason?: unknown) => void;
  resolve: (value: T) => void;
};

function createDeferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, reject, resolve };
}

function acceptedResponse(
  workspaceId: string | null = "workspace-1",
): AcceptProjectShareLinkResponse {
  return {
    project: {
      id: "project-1",
      workspace_id: workspaceId,
    },
    role: "editor",
  } as AcceptProjectShareLinkResponse;
}

function waitForUpdates() {
  return new Promise((resolve) => window.setTimeout(resolve, 0));
}

async function flushUpdates() {
  await act(async () => {
    await waitForUpdates();
  });
}

async function flushShareFlow() {
  await flushUpdates();
  await flushUpdates();
  await flushUpdates();
}

function renderPage(token = "share-token") {
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root: Root = createRoot(container);

  act(() => {
    root.render(<ShareProjectPage token={token} />);
  });

  return {
    container,
    rerender(nextToken = token) {
      act(() => {
        root.render(<ShareProjectPage token={nextToken} />);
      });
    },
    text() {
      return document.body.textContent ?? "";
    },
    unmount() {
      act(() => {
        root.unmount();
      });
      container.remove();
    },
  };
}

function buttonByText(text: string) {
  const button = Array.from(document.body.querySelectorAll("button")).find(
    (item) => item.textContent?.trim() === text,
  );
  if (!button) {
    throw new Error(`button not found: ${text}`);
  }
  return button;
}

describe("ShareProjectPage", () => {
  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    window.localStorage.clear();
    mocks.acceptProjectShareLink.mockReset();
    mocks.pathname = "/en/share/projects";
    mocks.replace.mockReset();
    mocks.toastSuccess.mockReset();
    mocks.authState.initialized = true;
    mocks.authState.session = { user: { id: "user-1" } };
  });

  afterEach(() => {
    document.body.innerHTML = "";
  });

  it("redirects unauthenticated users back to the share URL", async () => {
    mocks.authState.session = null;

    const view = renderPage("share-token");
    await flushUpdates();

    expect(mocks.acceptProjectShareLink).not.toHaveBeenCalled();
    expect(mocks.replace).toHaveBeenCalledTimes(1);
    const redirectUrl = new URL(
      mocks.replace.mock.calls[0]?.[0] as string,
      "https://app.test",
    );
    expect(redirectUrl.pathname).toBe("/en/login");
    expect(redirectUrl.searchParams.get("next")).toBe(
      "/en/share/projects?token=share-token",
    );

    view.unmount();
  });

  it("does not accept a duplicate token while the first request is pending", async () => {
    const deferred = createDeferred<AcceptProjectShareLinkResponse>();
    mocks.acceptProjectShareLink.mockReturnValue(deferred.promise);

    const view = renderPage("share-token");
    await flushUpdates();
    mocks.authState.session = { user: { id: "user-1" } };
    view.rerender("share-token");
    await flushUpdates();

    expect(mocks.acceptProjectShareLink).toHaveBeenCalledTimes(1);
    expect(mocks.acceptProjectShareLink).toHaveBeenCalledWith("share-token");
    expect(buttonByText("Continue").disabled).toBe(true);

    deferred.resolve(acceptedResponse());
    await flushShareFlow();

    expect(window.localStorage.getItem(selectedWorkspaceStorageKey)).toBe(
      "workspace-1",
    );
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Project access granted");
    expect(mocks.replace).toHaveBeenCalledWith(
      "/en/dashboard/content?projectId=project-1",
    );

    view.unmount();
  });

  it("surfaces failures and retries acceptance", async () => {
    mocks.acceptProjectShareLink
      .mockRejectedValueOnce(new Error("Invitation expired"))
      .mockResolvedValueOnce(acceptedResponse("workspace-2"));

    const view = renderPage("share-token");
    await flushShareFlow();

    expect(view.text()).toContain("This invitation could not be accepted.");
    expect(view.text()).toContain("Invitation expired");
    expect(buttonByText("Try again").disabled).toBe(false);
    expect(mocks.acceptProjectShareLink).toHaveBeenCalledTimes(1);
    expect(mocks.acceptProjectShareLink).toHaveBeenNthCalledWith(
      1,
      "share-token",
    );

    await act(async () => {
      buttonByText("Try again").click();
      await waitForUpdates();
    });
    await flushShareFlow();

    expect(mocks.acceptProjectShareLink).toHaveBeenCalledTimes(2);
    expect(mocks.acceptProjectShareLink).toHaveBeenNthCalledWith(
      2,
      "share-token",
    );
    expect(window.localStorage.getItem(selectedWorkspaceStorageKey)).toBe(
      "workspace-2",
    );
    expect(mocks.replace).toHaveBeenCalledWith(
      "/en/dashboard/content?projectId=project-1",
    );

    view.unmount();
  });

  it("disables manual acceptance when no token is present", async () => {
    const view = renderPage("");
    await flushUpdates();

    expect(mocks.acceptProjectShareLink).not.toHaveBeenCalled();
    expect(buttonByText("Continue").disabled).toBe(true);

    view.unmount();
  });
});

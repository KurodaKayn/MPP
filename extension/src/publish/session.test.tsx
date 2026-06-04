import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { BackendApiError } from "../backend/client";
import type { ExtensionSessionResponse } from "../backend/types";
import {
  SessionStatusCard,
  getSessionViewState,
  useExtensionSession,
} from "./session";

function createSession(username = "creator"): ExtensionSessionResponse {
  return {
    authenticated: true,
    user: {
      id: "user-1",
      username,
    },
  };
}

describe("getSessionViewState", () => {
  it("maps authenticated backend sessions to connected state", async () => {
    await expect(
      getSessionViewState(() => Promise.resolve(createSession("alice"))),
    ).resolves.toEqual({
      status: "authenticated",
      user: {
        id: "user-1",
        username: "alice",
      },
    });
  });

  it("maps 401 backend responses to expired state", async () => {
    await expect(
      getSessionViewState(() =>
        Promise.reject(
          new BackendApiError("token expired", {
            code: "unauthorized",
            status: 401,
          }),
        ),
      ),
    ).resolves.toMatchObject({
      status: "expired",
      message: "token expired",
    });
  });

  it("maps network errors to api unavailable state", async () => {
    await expect(
      getSessionViewState(() =>
        Promise.reject(
          new BackendApiError("Failed to fetch", {
            code: "network_error",
            status: 0,
          }),
        ),
      ),
    ).resolves.toMatchObject({
      status: "api_unavailable",
      message: "Failed to fetch",
    });
  });
});

describe("SessionStatusCard", () => {
  it("shows the authenticated user", () => {
    render(
      <SessionStatusCard
        state={{
          status: "authenticated",
          user: {
            id: "user-1",
            username: "creator",
          },
        }}
        onOpenLogin={vi.fn()}
        onRetry={vi.fn()}
      />,
    );

    expect(screen.getByText("MPP Session")).toBeInTheDocument();
    expect(screen.getByText("creator")).toBeInTheDocument();
    expect(screen.getByText("connected")).toBeInTheDocument();
  });

  it("opens the MPP login page from expired state", async () => {
    const openLogin = vi.fn();

    render(
      <SessionStatusCard
        state={{
          status: "expired",
          message: "token expired",
        }}
        onOpenLogin={openLogin}
        onRetry={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /open mpp/i }));

    expect(openLogin).toHaveBeenCalledOnce();
  });

  it("shows backend retry action when the API is unavailable", async () => {
    const retry = vi.fn();

    render(
      <SessionStatusCard
        state={{
          status: "api_unavailable",
          message: "Failed to fetch",
        }}
        onOpenLogin={vi.fn()}
        onRetry={retry}
      />,
    );

    expect(screen.getByText("Failed to fetch")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /retry/i }));

    expect(retry).toHaveBeenCalledOnce();
  });
});

describe("useExtensionSession", () => {
  it("checks session on mount", async () => {
    function Harness({
      loadSession,
    }: {
      loadSession: () => Promise<ExtensionSessionResponse>;
    }) {
      const { state } = useExtensionSession(loadSession);
      return (
        <SessionStatusCard
          state={state}
          onOpenLogin={vi.fn()}
          onRetry={vi.fn()}
        />
      );
    }
    const loadSession = vi.fn().mockResolvedValue(createSession("creator"));

    render(<Harness loadSession={loadSession} />);

    expect(screen.getByText("checking")).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByText("creator")).toBeInTheDocument(),
    );
    expect(loadSession).toHaveBeenCalledOnce();
  });
});

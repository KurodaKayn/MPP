// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Workspace } from "@/lib/dashboard/api";
import {
  DashboardWorkspaceProvider,
  useDashboardWorkspaceSelection,
} from "./use-dashboard-workspace-selection";

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

const mocks = vi.hoisted(() => ({
  createWorkspace: vi.fn(),
  getWorkspaces: vi.fn(),
}));

vi.mock("@/lib/dashboard/api", () => ({
  createWorkspace: mocks.createWorkspace,
  getWorkspaces: mocks.getWorkspaces,
}));

type Selection = ReturnType<typeof useDashboardWorkspaceSelection>;

function workspace(overrides: Partial<Workspace>): Workspace {
  return {
    created_at: "2026-06-01T00:00:00.000Z",
    id: "workspace-1",
    name: "Personal",
    owner_user_id: "user-1",
    role: "owner",
    slug: "personal",
    status: "active",
    updated_at: "2026-06-01T00:00:00.000Z",
    ...overrides,
  };
}

function flushPromises() {
  return new Promise((resolve) => window.setTimeout(resolve, 0));
}

function renderSelection() {
  let selection: Selection | undefined;
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);

  function Harness() {
    selection = useDashboardWorkspaceSelection();
    return null;
  }

  act(() => {
    root.render(
      <DashboardWorkspaceProvider>
        <Harness />
      </DashboardWorkspaceProvider>,
    );
  });

  return {
    getSelection() {
      if (!selection) {
        throw new Error("Selection did not render.");
      }
      return selection;
    },
    unmount() {
      act(() => {
        root.unmount();
      });
      container.remove();
    },
  };
}

describe("useDashboardWorkspaceSelection", () => {
  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    window.localStorage.clear();
    mocks.createWorkspace.mockReset();
    mocks.getWorkspaces.mockReset();
  });

  it("restores the last selected workspace when it is still available", async () => {
    window.localStorage.setItem(
      "mpp.dashboard.selectedWorkspaceId",
      "workspace-2",
    );
    mocks.getWorkspaces.mockResolvedValue({
      items: [
        workspace({ id: "workspace-1", name: "Personal" }),
        workspace({ id: "workspace-2", name: "Team" }),
      ],
    });

    const view = renderSelection();
    await act(async () => {
      await flushPromises();
    });

    expect(view.getSelection().selectedWorkspaceId).toBe("workspace-2");
    expect(view.getSelection().selectedWorkspace?.name).toBe("Team");

    view.unmount();
  });

  it("selects and stores a newly created workspace", async () => {
    const created = workspace({
      id: "workspace-2",
      name: "Team",
      slug: "team",
    });
    mocks.getWorkspaces.mockResolvedValue({
      items: [workspace({ id: "workspace-1", name: "Personal" })],
    });
    mocks.createWorkspace.mockResolvedValue(created);

    const view = renderSelection();
    await act(async () => {
      await flushPromises();
    });

    await act(async () => {
      await view.getSelection().createWorkspace({
        name: "Team",
        slug: "team",
      });
    });

    expect(mocks.createWorkspace).toHaveBeenCalledWith({
      name: "Team",
      slug: "team",
    });
    expect(view.getSelection().selectedWorkspaceId).toBe("workspace-2");
    expect(view.getSelection().workspaces[0]?.name).toBe("Team");
    expect(
      window.localStorage.getItem("mpp.dashboard.selectedWorkspaceId"),
    ).toBe("workspace-2");

    view.unmount();
  });
});

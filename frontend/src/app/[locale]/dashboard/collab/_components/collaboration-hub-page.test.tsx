// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ProjectCollaborator, ProjectListItem } from "@/lib/dashboard/api";

import { CollaborationHubPage } from "./collaboration-hub-page";

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

const mocks = vi.hoisted(() => {
  const translate = (key: string, values?: Record<string, unknown>) => {
    const translations: Record<string, string> = {
      "collab.hub.error.defaultMessage": "Unable to load projects",
      "project.delete.confirm": `Delete "${String(values?.title ?? "")}"?`,
      "project.delete.failed": "Unable to delete project",
      "project.delete.label": "Delete project",
      "project.delete.noPermission": "No delete permission",
      "project.delete.retryLater": "Please try again later.",
      "project.delete.success": "Project deleted",
      "workspace.empty": "No workspace",
    };
    return translations[key] ?? key;
  };

  return {
    deleteDashboardProject: vi.fn(),
    getDashboardProjects: vi.fn(),
    getProjectCollaborators: vi.fn(),
    getWorkspaceProjects: vi.fn(),
    toastError: vi.fn(),
    toastSuccess: vi.fn(),
    translate,
    workspaceSelection: {
      createWorkspace: vi.fn(),
      error: "",
      isCreatingWorkspace: false,
      isLoading: false,
      reloadWorkspaces: vi.fn(),
      selectWorkspace: vi.fn(),
      selectedWorkspace: {
        id: "workspace-1",
        role: "owner",
      },
      selectedWorkspaceId: "workspace-1",
      workspaces: [],
    },
  };
});

vi.mock("@/lib/dashboard/api", () => ({
  deleteDashboardProject: mocks.deleteDashboardProject,
  getDashboardProjects: mocks.getDashboardProjects,
  getProjectCollaborators: mocks.getProjectCollaborators,
  getWorkspaceProjects: mocks.getWorkspaceProjects,
}));

vi.mock("@/lib/i18n/client", () => ({
  useAppLocale: () => "en",
  useTranslation: () => ({ t: mocks.translate }),
}));

vi.mock("sonner", () => ({
  toast: {
    error: mocks.toastError,
    success: mocks.toastSuccess,
  },
}));

vi.mock("../../_components/publication-platforms", () => ({
  PlatformIconRow: () => null,
}));

vi.mock("../../_components/workspace-switcher", () => ({
  WorkspaceSwitcher: () => null,
}));

vi.mock("../../_hooks/use-dashboard-workspace-selection", () => ({
  useDashboardWorkspaceSelection: () => mocks.workspaceSelection,
}));

function project(overrides: Partial<ProjectListItem>): ProjectListItem {
  return {
    access_source: "owner",
    created_at: "2026-01-01T00:00:00.000Z",
    id: "project-1",
    publications: [],
    role: "owner",
    status: "ready",
    title: "Project",
    updated_at: "2026-01-02T00:00:00.000Z",
    user_id: "user-1",
    workspace_id: "workspace-1",
    ...overrides,
  };
}

function collaborator(
  overrides: Partial<ProjectCollaborator>,
): ProjectCollaborator {
  return {
    created_at: "2026-01-03T00:00:00.000Z",
    created_by: "user-1",
    email: "teammate@example.com",
    project_id: "project-1",
    role: "editor",
    user_id: "user-2",
    username: "teammate",
    ...overrides,
  };
}

function waitForUpdates() {
  return new Promise((resolve) => window.setTimeout(resolve, 0));
}

async function flushUpdates() {
  await act(async () => {
    await waitForUpdates();
  });
}

async function flushPageLoad() {
  await flushUpdates();
  await flushUpdates();
  await flushUpdates();
}

function renderPage() {
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);

  act(() => {
    root.render(<CollaborationHubPage />);
  });

  return {
    container,
    text() {
      return container.textContent ?? "";
    },
    unmount() {
      act(() => {
        root.unmount();
      });
      container.remove();
    },
  };
}

function deleteButtons(container: HTMLElement) {
  return Array.from(container.querySelectorAll("button")).filter((button) =>
    button.textContent?.includes("Delete project"),
  );
}

describe("CollaborationHubPage project deletion", () => {
  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    mocks.deleteDashboardProject.mockReset();
    mocks.getDashboardProjects.mockReset();
    mocks.getProjectCollaborators.mockReset();
    mocks.getWorkspaceProjects.mockReset();
    mocks.toastError.mockReset();
    mocks.toastSuccess.mockReset();
    vi.stubGlobal(
      "confirm",
      vi.fn(() => true),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("removes a deleted project from shared-by-me and workspace lists", async () => {
    const ownedProject = project({
      id: "owned-project",
      title: "Owned project",
    });
    const sharedProject = project({
      access_source: "direct_share",
      id: "shared-project",
      role: "editor",
      title: "Shared project",
      workspace_id: undefined,
    });
    mocks.getDashboardProjects.mockResolvedValue({
      items: [ownedProject, sharedProject],
    });
    mocks.getProjectCollaborators.mockResolvedValue({
      items: [collaborator({ project_id: ownedProject.id })],
    });
    mocks.getWorkspaceProjects.mockResolvedValue({
      items: [ownedProject],
    });
    mocks.deleteDashboardProject.mockResolvedValue(undefined);

    const view = renderPage();
    await flushPageLoad();

    expect(view.text()).toContain("Owned project");
    expect(view.text()).toContain("Shared project");
    expect(deleteButtons(view.container)).toHaveLength(2);

    await act(async () => {
      deleteButtons(view.container)[0]?.click();
      await waitForUpdates();
    });

    expect(mocks.deleteDashboardProject).toHaveBeenCalledWith("owned-project");
    expect(view.text()).not.toContain("Owned project");
    expect(view.text()).toContain("Shared project");
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Project deleted");

    view.unmount();
  });
});

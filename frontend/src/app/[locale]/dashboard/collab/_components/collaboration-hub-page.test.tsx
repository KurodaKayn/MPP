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
      "project.delete.cancel": "Cancel",
      "project.delete.failed": "Unable to delete project",
      "project.delete.label": "Delete project",
      "project.delete.noPermission": "No delete permission",
      "project.delete.retryLater": "Please try again later.",
      "project.delete.submit": "Delete",
      "project.delete.success": "Project deleted",
      "project.delete.title": "Delete project",
      "workspace.empty": "No workspace",
    };
    return translations[key] ?? key;
  };

  return {
    deleteDashboardProject: vi.fn(),
    getDashboardProjects: vi.fn(),
    getOwnedProjectCollaboratorSummaries: vi.fn(),
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
  getOwnedProjectCollaboratorSummaries:
    mocks.getOwnedProjectCollaboratorSummaries,
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

function deleteButtons(container: HTMLElement) {
  return Array.from(container.querySelectorAll("button")).filter((button) =>
    button.textContent?.includes("Delete project"),
  );
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

describe("CollaborationHubPage project deletion", () => {
  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    mocks.deleteDashboardProject.mockReset();
    mocks.getDashboardProjects.mockReset();
    mocks.getOwnedProjectCollaboratorSummaries.mockReset();
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
    mocks.getOwnedProjectCollaboratorSummaries.mockResolvedValue({
      items: [
        {
          collaborator_count: 1,
          collaborators: [collaborator({ project_id: ownedProject.id })],
          project_id: ownedProject.id,
        },
      ],
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
    expect(mocks.getProjectCollaborators).not.toHaveBeenCalled();

    await act(async () => {
      deleteButtons(view.container)[0]?.click();
      await waitForUpdates();
    });
    expect(globalThis.confirm).not.toHaveBeenCalled();
    expect(view.text()).toContain('Delete "Owned project"?');
    expect(mocks.deleteDashboardProject).not.toHaveBeenCalled();

    await act(async () => {
      buttonByText("Delete").click();
      await waitForUpdates();
    });

    expect(mocks.deleteDashboardProject).toHaveBeenCalledWith("owned-project", {
      workspaceId: "workspace-1",
    });
    expect(view.text()).not.toContain("Owned project");
    expect(view.text()).toContain("Shared project");
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Project deleted");

    view.unmount();
  });

  it("keeps shared-with-me projects visible when collaborator summaries fail", async () => {
    const sharedProject = project({
      access_source: "direct_share",
      id: "shared-project",
      role: "viewer",
      title: "Shared project",
      workspace_id: undefined,
    });
    mocks.getDashboardProjects.mockResolvedValue({
      items: [sharedProject],
    });
    mocks.getOwnedProjectCollaboratorSummaries.mockRejectedValue(
      new Error("Unable to load collaborator summaries"),
    );
    mocks.getWorkspaceProjects.mockResolvedValue({
      items: [],
    });

    const view = renderPage();
    await flushPageLoad();

    expect(view.text()).toContain("Shared project");
    expect(view.text()).toContain("Unable to load collaborator summaries");

    view.unmount();
  });

  it("deletes personal owned projects without inheriting the selected workspace", async () => {
    const personalProject = project({
      id: "personal-project",
      title: "Personal project",
      workspace_id: null,
    });
    mocks.getDashboardProjects.mockResolvedValue({
      items: [personalProject],
    });
    mocks.getOwnedProjectCollaboratorSummaries.mockResolvedValue({
      items: [
        {
          collaborator_count: 1,
          collaborators: [collaborator({ project_id: personalProject.id })],
          project_id: personalProject.id,
        },
      ],
    });
    mocks.getWorkspaceProjects.mockResolvedValue({
      items: [],
    });
    mocks.deleteDashboardProject.mockResolvedValue(undefined);

    const view = renderPage();
    await flushPageLoad();

    await act(async () => {
      deleteButtons(view.container)[0]?.click();
      await waitForUpdates();
    });
    await act(async () => {
      buttonByText("Delete").click();
      await waitForUpdates();
    });

    expect(mocks.deleteDashboardProject).toHaveBeenCalledWith(
      "personal-project",
      { workspaceId: null },
    );
    expect(view.text()).not.toContain("Personal project");

    view.unmount();
  });
});

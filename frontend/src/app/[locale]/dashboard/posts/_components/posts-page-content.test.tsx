// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ProjectListItem } from "@/lib/dashboard/api";

import { PostsPageContent } from "./posts-page-content";

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

const mocks = vi.hoisted(() => {
  const translate = (key: string, values?: Record<string, unknown>) => {
    const translations: Record<string, string> = {
      "project.delete.confirm": `Delete "${String(values?.title ?? "")}"?`,
      "project.delete.cancel": "Cancel",
      "project.delete.failed": "Unable to delete project",
      "project.delete.label": "Delete project",
      "project.delete.noPermission": "No delete permission",
      "project.delete.retryLater": "Please try again later.",
      "project.delete.submit": "Delete",
      "project.delete.success": "Project deleted",
      "project.delete.title": "Delete project",
      "posts.card.none": "None",
      "posts.error.defaultMessage": "Unable to load posts",
      "workspace.empty": "No workspace",
    };
    return translations[key] ?? key;
  };

  return {
    deleteDashboardProject: vi.fn(),
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
}

function renderPage() {
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);

  act(() => {
    root.render(<PostsPageContent />);
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

describe("PostsPageContent project deletion", () => {
  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    mocks.deleteDashboardProject.mockReset();
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

  it("removes the project card after a confirmed delete succeeds", async () => {
    mocks.getWorkspaceProjects.mockResolvedValue({
      items: [
        project({ id: "project-1", title: "First project" }),
        project({ id: "project-2", title: "Second project" }),
      ],
    });
    mocks.deleteDashboardProject.mockResolvedValue(undefined);

    const view = renderPage();
    await flushPageLoad();

    expect(view.text()).toContain("First project");
    expect(view.text()).toContain("Second project");

    await act(async () => {
      deleteButtons(view.container)[0]?.click();
      await waitForUpdates();
    });
    expect(globalThis.confirm).not.toHaveBeenCalled();
    expect(view.text()).toContain('Delete "First project"?');
    expect(mocks.deleteDashboardProject).not.toHaveBeenCalled();

    await act(async () => {
      buttonByText("Delete").click();
      await waitForUpdates();
    });

    expect(mocks.deleteDashboardProject).toHaveBeenCalledWith("project-1");
    expect(view.text()).not.toContain("First project");
    expect(view.text()).toContain("Second project");
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Project deleted");

    view.unmount();
  });

  it("keeps the project card when delete fails", async () => {
    mocks.getWorkspaceProjects.mockResolvedValue({
      items: [project({ id: "project-1", title: "First project" })],
    });
    mocks.deleteDashboardProject.mockRejectedValue(new Error("Still active"));

    const view = renderPage();
    await flushPageLoad();

    await act(async () => {
      deleteButtons(view.container)[0]?.click();
      await waitForUpdates();
    });
    expect(globalThis.confirm).not.toHaveBeenCalled();

    await act(async () => {
      buttonByText("Delete").click();
      await waitForUpdates();
    });

    expect(view.text()).toContain("First project");
    expect(mocks.toastError).toHaveBeenCalledWith("Unable to delete project", {
      description: "Still active",
    });

    view.unmount();
  });
});

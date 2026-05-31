// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useContentPageStore } from "../_stores/content-page-store";
import { useContentPageController } from "./use-content-page-controller";

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

const mocks = vi.hoisted(() => ({
  createDashboardProject: vi.fn(),
  getDashboardProject: vi.fn(),
  getProjectPublications: vi.fn(),
  publishProject: vi.fn(),
  push: vi.fn(),
  refresh: vi.fn(),
  replace: vi.fn(),
  toastError: vi.fn(),
  toastSuccess: vi.fn(),
  syncProjectPrepublish: vi.fn(),
  updateDashboardProject: vi.fn(),
  waitForProjectPublications: vi.fn(),
}));

vi.mock("@/lib/dashboard/api", () => ({
  createDashboardProject: mocks.createDashboardProject,
  getDashboardProject: mocks.getDashboardProject,
  getProjectPublications: mocks.getProjectPublications,
  publishProject: mocks.publishProject,
  syncProjectPrepublish: mocks.syncProjectPrepublish,
  updateDashboardProject: mocks.updateDashboardProject,
  waitForProjectPublications: mocks.waitForProjectPublications,
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mocks.push,
    refresh: mocks.refresh,
    replace: mocks.replace,
  }),
}));

vi.mock("sonner", () => ({
  toast: {
    error: mocks.toastError,
    success: mocks.toastSuccess,
  },
}));

type Controller = ReturnType<typeof useContentPageController>;

function renderController(projectId?: string) {
  let controller: Controller | undefined;
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);

  function Harness() {
    controller = useContentPageController(projectId);
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

describe("useContentPageController", () => {
  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    mocks.createDashboardProject.mockReset();
    mocks.getDashboardProject.mockReset();
    mocks.getProjectPublications.mockReset();
    mocks.publishProject.mockReset();
    mocks.push.mockReset();
    mocks.replace.mockReset();
    mocks.refresh.mockReset();
    mocks.toastError.mockReset();
    mocks.toastSuccess.mockReset();
    mocks.syncProjectPrepublish.mockReset();
    mocks.updateDashboardProject.mockReset();
    mocks.waitForProjectPublications.mockReset();
    useContentPageStore.getState().resetForCreate();
  });

  it("reports loading before the current edit project has loaded", () => {
    mocks.getDashboardProject.mockImplementation(() => new Promise(() => {}));
    useContentPageStore.setState({
      content: {
        firstImageSrc: "",
        html: "<p>Old body</p>",
        text: "Old body",
      },
      loadedProjectId: "old-project",
      selectedPlatforms: ["wechat"],
      title: "Old title",
    });

    const view = renderController("new-project");

    expect(view.getController().isLoading).toBe(true);
    expect(view.getController().canPublish).toBe(false);
    expect(mocks.getDashboardProject).toHaveBeenCalledWith("new-project");

    view.unmount();
  });

  it("syncs prepublish drafts with platform-specific formats", async () => {
    mocks.createDashboardProject.mockResolvedValue({ id: "project-1" });
    mocks.syncProjectPrepublish.mockResolvedValue({
      items: [
        {
          adapted_content: {
            format: "html",
            html: "<p>Rendered body</p>",
            source_revision: "2026-05-30T12:00:00.000Z",
          },
          enabled: true,
          platform: "wechat",
          updated_at: "2026-05-30T12:00:00.000Z",
        },
        {
          adapted_content: {
            format: "markdown",
            markdown: "Rendered body",
            source_revision: "2026-05-30T12:00:00.000Z",
          },
          enabled: true,
          platform: "zhihu",
          updated_at: "2026-05-30T12:00:00.000Z",
        },
        {
          adapted_content: {
            format: "text",
            source_revision: "2026-05-30T12:00:00.000Z",
            text: "Rendered body",
          },
          enabled: true,
          platform: "x",
          updated_at: "2026-05-30T12:00:00.000Z",
        },
      ],
      project_id: "project-1",
    });
    const view = renderController();

    act(() => {
      useContentPageStore.setState({
        content: {
          firstImageSrc: "",
          html: "<p>Rendered body</p>",
          text: "Rendered body",
        },
        selectedPlatforms: ["wechat", "zhihu", "x"],
        title: "Post title",
      });
    });

    await act(async () => {
      await view.getController().syncPrepublish();
    });

    const state = useContentPageStore.getState();
    expect(state.prepublishDrafts.wechat).toMatchObject({
      format: "html",
      raw: "<p>Rendered body</p>",
    });
    expect(state.prepublishDrafts.zhihu).toMatchObject({
      format: "markdown",
      raw: "Rendered body",
    });
    expect(state.prepublishDrafts.x).toMatchObject({
      format: "text",
      raw: "Rendered body",
    });
    expect(state.isSyncingPrepublish).toBe(false);
    expect(mocks.createDashboardProject).toHaveBeenCalledWith({
      platforms: ["wechat", "zhihu", "x"],
      source_content: "<p>Rendered body</p>",
      summary: "Rendered body",
      title: "Post title",
    });
    expect(mocks.syncProjectPrepublish).toHaveBeenCalledWith("project-1", {
      platforms: ["wechat", "zhihu", "x"],
    });
    expect(mocks.toastSuccess).toHaveBeenCalledWith("已同步到预发布", {
      description: "平台草稿已由后端适配并保存。",
    });
    expect(mocks.replace).toHaveBeenCalledWith("/dashboard/content/project-1");

    view.unmount();
  });

  it("does not sync drafts when no platform is selected", () => {
    const view = renderController();

    act(() => {
      useContentPageStore.setState({
        content: {
          firstImageSrc: "",
          html: "<p>Rendered body</p>",
          text: "Rendered body",
        },
        selectedPlatforms: [],
        title: "Post title",
      });
    });

    act(() => {
      view.getController().syncPrepublish();
    });

    expect(useContentPageStore.getState().prepublishDrafts).toEqual({});
    expect(mocks.toastError).toHaveBeenCalledWith("请选择发布平台", {
      description: "在底部发布渠道中勾选至少一个平台。",
    });
    expect(mocks.toastSuccess).not.toHaveBeenCalled();

    view.unmount();
  });

  it("saves the current platform selection before publishing", async () => {
    mocks.getDashboardProject.mockResolvedValue({
      created_at: "2026-05-30T12:00:00.000Z",
      id: "project-1",
      publications: [
        { enabled: true, id: "pub-1", platform: "wechat", status: "adapted" },
        { enabled: true, id: "pub-2", platform: "zhihu", status: "adapted" },
      ],
      source_content: "<p>Rendered body</p>",
      status: "ready",
      title: "Post title",
      updated_at: "2026-05-30T12:00:00.000Z",
      user_id: "user-1",
    });
    mocks.getProjectPublications.mockResolvedValue({
      items: [
        {
          adapted_content: {
            format: "html",
            html: "<p>Rendered body</p>",
            source_revision: "2026-05-30T12:00:00.000Z",
          },
          enabled: true,
          platform: "wechat",
          updated_at: "2026-05-30T12:00:00.000Z",
        },
        {
          adapted_content: {
            format: "markdown",
            markdown: "Rendered body",
            source_revision: "2026-05-30T12:00:00.000Z",
          },
          enabled: true,
          platform: "zhihu",
          updated_at: "2026-05-30T12:00:00.000Z",
        },
      ],
      project_id: "project-1",
    });
    mocks.updateDashboardProject.mockResolvedValue({
      id: "project-1",
    });
    mocks.publishProject.mockResolvedValue({
      job_id: "job-1",
      status: "publishing",
    });
    mocks.waitForProjectPublications.mockResolvedValue({
      items: [
        {
          adapted_content: {},
          config: {},
          created_at: "2026-05-30T12:00:00.000Z",
          enabled: true,
          id: "pub-2",
          platform: "zhihu",
          publish_url: "https://example.com/zhihu",
          retry_count: 0,
          status: "published",
          updated_at: "2026-05-30T12:00:00.000Z",
        },
      ],
      project_id: "project-1",
    });

    const view = renderController("project-1");

    await act(async () => {
      await Promise.resolve();
    });

    act(() => {
      useContentPageStore.setState({
        content: {
          firstImageSrc: "",
          html: "<p>Rendered body</p>",
          text: "Rendered body",
        },
        prepublishDrafts: {
          zhihu: {
            format: "markdown",
            raw: "Rendered body",
            syncedAt: "2026-05-30T12:00:00.000Z",
          },
        },
        selectedPlatforms: ["zhihu"],
        title: "Post title",
      });
    });

    await act(async () => {
      await view.getController().publish();
    });

    expect(mocks.updateDashboardProject).toHaveBeenCalledWith("project-1", {
      cover_image_url: undefined,
      platforms: ["zhihu"],
      source_content: "<p>Rendered body</p>",
      summary: "Rendered body",
      title: "Post title",
    });
    expect(mocks.publishProject).toHaveBeenCalledWith("project-1", "zhihu");
    expect(mocks.waitForProjectPublications).toHaveBeenCalledWith("project-1", [
      "zhihu",
    ]);
    expect(mocks.toastSuccess).toHaveBeenCalledWith("修改并发布完成", {
      description: "已发布到 知乎。",
    });

    view.unmount();
  });
});

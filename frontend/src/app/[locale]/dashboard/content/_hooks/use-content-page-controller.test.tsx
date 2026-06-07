// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ContentValue } from "@/lib/content/types";
import type { Workspace } from "@/lib/dashboard/api";
import { useContentPageStore } from "../_stores/content-page-store";
import { useContentPageController } from "./use-content-page-controller";

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

const mocks = vi.hoisted(() => ({
  cancelBrowserSession: vi.fn(),
  createDashboardProject: vi.fn(),
  createWorkspaceProject: vi.fn(),
  getBrandProfiles: vi.fn(),
  getContentTemplates: vi.fn(),
  getDashboardProject: vi.fn(),
  getProjectPublications: vi.fn(),
  getWorkspaceBrandProfiles: vi.fn(),
  getWorkspaceContentTemplates: vi.fn(),
  publishProject: vi.fn(),
  push: vi.fn(),
  refresh: vi.fn(),
  replace: vi.fn(),
  saveDashboardProjectContent: vi.fn(),
  saveDashboardProjectPlatforms: vi.fn(),
  startDouyinPublishSession: vi.fn(),
  toastError: vi.fn(),
  toastSuccess: vi.fn(),
  syncProjectPrepublish: vi.fn(),
  updateDashboardProject: vi.fn(),
  waitForProjectPublications: vi.fn(),
}));

vi.mock("@/lib/i18n/client", () => ({
  useAppLocale: () => "en",
  useTranslation: () => ({
    t: (key: string, options?: any) => {
      if (key === "publish.publishedTo") {
        return `Published to ${options.platforms}.`;
      }
      if (key === "platforms.zhihu") {
        return "Zhihu";
      }
      return key;
    },
  }),
}));

vi.mock("@/lib/dashboard/api", () => ({
  cancelBrowserSession: mocks.cancelBrowserSession,
  createDashboardProject: mocks.createDashboardProject,
  createWorkspaceProject: mocks.createWorkspaceProject,
  getBrandProfiles: mocks.getBrandProfiles,
  getContentTemplates: mocks.getContentTemplates,
  getDashboardProject: mocks.getDashboardProject,
  getProjectPublications: mocks.getProjectPublications,
  getWorkspaceBrandProfiles: mocks.getWorkspaceBrandProfiles,
  getWorkspaceContentTemplates: mocks.getWorkspaceContentTemplates,
  publishProject: mocks.publishProject,
  saveDashboardProjectContent: mocks.saveDashboardProjectContent,
  saveDashboardProjectPlatforms: mocks.saveDashboardProjectPlatforms,
  startDouyinPublishSession: mocks.startDouyinPublishSession,
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
type WorkspaceContext = Parameters<typeof useContentPageController>[1];

function flushPromises() {
  return new Promise((resolve) => window.setTimeout(resolve, 0));
}

function renderController(
  projectId?: string,
  workspaceContext?: WorkspaceContext,
) {
  let controller: Controller | undefined;
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);

  function Harness() {
    controller = useContentPageController(projectId, workspaceContext);
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
    mocks.cancelBrowserSession.mockReset();
    mocks.createDashboardProject.mockReset();
    mocks.createWorkspaceProject.mockReset();
    mocks.getBrandProfiles.mockReset();
    mocks.getContentTemplates.mockReset();
    mocks.getDashboardProject.mockReset();
    mocks.getProjectPublications.mockReset();
    mocks.getWorkspaceBrandProfiles.mockReset();
    mocks.getWorkspaceContentTemplates.mockReset();
    mocks.publishProject.mockReset();
    mocks.push.mockReset();
    mocks.replace.mockReset();
    mocks.refresh.mockReset();
    mocks.saveDashboardProjectContent.mockReset();
    mocks.saveDashboardProjectPlatforms.mockReset();
    mocks.startDouyinPublishSession.mockReset();
    mocks.toastError.mockReset();
    mocks.toastSuccess.mockReset();
    mocks.syncProjectPrepublish.mockReset();
    mocks.updateDashboardProject.mockReset();
    mocks.waitForProjectPublications.mockReset();
    mocks.getBrandProfiles.mockResolvedValue({ items: [] });
    mocks.getContentTemplates.mockResolvedValue({ items: [] });
    mocks.getWorkspaceBrandProfiles.mockResolvedValue({ items: [] });
    mocks.getWorkspaceContentTemplates.mockResolvedValue({ items: [] });
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
    expect(view.getController().publishing.canPublish).toBe(false);
    expect(mocks.getDashboardProject).toHaveBeenCalledWith("new-project");

    view.unmount();
  });

  it("confirms before applying a template over existing draft content", async () => {
    mocks.getContentTemplates.mockResolvedValue({
      items: [
        {
          created_at: "2026-06-06T12:00:00.000Z",
          default_platforms: ["wechat", "zhihu"],
          description: "",
          id: "template-1",
          name: "Launch template",
          platform_config: {},
          scope: "personal",
          source_template: "<p>Template body</p>",
          tags: [],
          title_template: "Template title",
          updated_at: "2026-06-06T12:00:00.000Z",
        },
      ],
    });
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);
    const view = renderController();

    await act(async () => {
      await flushPromises();
    });
    act(() => {
      useContentPageStore.setState({
        content: {
          firstImageSrc: "",
          html: "<p>Custom body</p>",
          text: "Custom body",
        },
        selectedPlatforms: ["x"],
        title: "Custom title",
      });
    });

    act(() => {
      view.getController().setup.onTemplateChange("template-1");
    });

    expect(confirmSpy).toHaveBeenCalledWith("content.setup.replaceConfirm");
    expect(useContentPageStore.getState().title).toBe("Custom title");
    expect(useContentPageStore.getState().content.html).toBe(
      "<p>Custom body</p>",
    );
    expect(useContentPageStore.getState().selectedPlatforms).toEqual(["x"]);
    expect(view.getController().setup.selectedTemplateId).toBe("");

    confirmSpy.mockReturnValue(true);
    act(() => {
      view.getController().setup.onTemplateChange("template-1");
    });

    expect(useContentPageStore.getState().title).toBe("Template title");
    expect(useContentPageStore.getState().content.html).toBe(
      "<p>Template body</p>",
    );
    expect(useContentPageStore.getState().selectedPlatforms).toEqual([
      "wechat",
      "zhihu",
    ]);
    expect(view.getController().setup.selectedTemplateId).toBe("template-1");

    confirmSpy.mockRestore();
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
        {
          adapted_content: {
            format: "text",
            source_revision: "2026-05-30T12:00:00.000Z",
            text: "Rendered body",
          },
          enabled: true,
          platform: "douyin",
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
        selectedPlatforms: ["wechat", "zhihu", "x", "douyin"],
        title: "Post title",
      });
    });

    await act(async () => {
      await view.getController().prepublish.onSync();
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
    expect(state.prepublishDrafts.douyin).toMatchObject({
      format: "text",
      raw: "Rendered body",
    });
    expect(state.isSyncingPrepublish).toBe(false);
    expect(mocks.createDashboardProject).toHaveBeenCalledWith({
      platforms: ["wechat", "zhihu", "x", "douyin"],
      source_content: "<p>Rendered body</p>",
      summary: "Rendered body",
      title: "Post title",
    });
    expect(mocks.syncProjectPrepublish).toHaveBeenCalledWith("project-1", {
      platforms: ["wechat", "zhihu", "x", "douyin"],
    });
    expect(mocks.toastSuccess).toHaveBeenCalledWith("project.syncSuccess", {
      description: "project.syncDesc",
    });
    expect(mocks.replace).toHaveBeenCalledWith("/dashboard/content/project-1");

    view.unmount();
  });

  it("does not restore syncing publications as publishable drafts", async () => {
    mocks.createDashboardProject.mockResolvedValue({ id: "project-1" });
    mocks.syncProjectPrepublish.mockResolvedValue({
      items: [
        {
          adapted_content: {
            format: "html",
            html: "<p>Old body</p>",
            source_revision: "2026-05-30T12:00:00.000Z",
          },
          enabled: true,
          platform: "wechat",
          status: "syncing",
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
          status: "draft",
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
        selectedPlatforms: ["wechat", "zhihu"],
        title: "Post title",
      });
    });

    await act(async () => {
      await view.getController().prepublish.onSync();
    });

    const state = useContentPageStore.getState();
    expect(state.prepublishDrafts.wechat).toBeUndefined();
    expect(state.prepublishDrafts.zhihu).toMatchObject({
      format: "markdown",
      raw: "Rendered body",
    });

    view.unmount();
  });

  it("creates prepublish drafts in the selected workspace", async () => {
    const selectedWorkspace: Workspace = {
      created_at: "2026-06-05T12:00:00.000Z",
      id: "workspace-1",
      name: "Team Workspace",
      owner_user_id: "user-1",
      role: "member",
      slug: "team-workspace",
      status: "active",
      updated_at: "2026-06-05T12:00:00.000Z",
    };

    mocks.createWorkspaceProject.mockResolvedValue({ id: "project-1" });
    mocks.syncProjectPrepublish.mockResolvedValue({
      items: [
        {
          adapted_content: {
            format: "html",
            html: "<p>Workspace body</p>",
            source_revision: "2026-06-05T12:00:00.000Z",
          },
          enabled: true,
          platform: "wechat",
          updated_at: "2026-06-05T12:00:00.000Z",
        },
      ],
      project_id: "project-1",
    });
    const view = renderController(undefined, {
      requiresWorkspace: true,
      selectedWorkspace,
    });

    act(() => {
      useContentPageStore.setState({
        content: {
          firstImageSrc: "",
          html: "<p>Workspace body</p>",
          text: "Workspace body",
        },
        selectedPlatforms: ["wechat"],
        title: "Workspace post",
      });
    });

    await act(async () => {
      await view.getController().prepublish.onSync();
    });

    expect(mocks.createWorkspaceProject).toHaveBeenCalledWith("workspace-1", {
      platforms: ["wechat"],
      source_content: "<p>Workspace body</p>",
      summary: "Workspace body",
      title: "Workspace post",
    });
    expect(mocks.createDashboardProject).not.toHaveBeenCalled();
    expect(mocks.syncProjectPrepublish).toHaveBeenCalledWith("project-1", {
      platforms: ["wechat"],
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
      view.getController().prepublish.onSync();
    });

    expect(useContentPageStore.getState().prepublishDrafts).toEqual({});
    expect(mocks.toastError).toHaveBeenCalledWith(
      "project.selectPlatformTitle",
      {
        description: "project.selectPlatformDesc",
      },
    );
    expect(mocks.toastSuccess).not.toHaveBeenCalled();

    view.unmount();
  });

  it("prepares pending editor media before saving project changes", async () => {
    mocks.getDashboardProject.mockResolvedValue({
      created_at: "2026-05-30T12:00:00.000Z",
      id: "project-1",
      publications: [
        { enabled: true, id: "pub-1", platform: "wechat", status: "draft" },
      ],
      role: "owner",
      source_content: "<p>Old body</p>",
      status: "ready",
      title: "Old title",
      updated_at: "2026-05-30T12:00:00.000Z",
      user_id: "user-1",
    });
    mocks.getProjectPublications.mockResolvedValue({
      items: [],
      project_id: "project-1",
    });
    mocks.updateDashboardProject.mockResolvedValue({ id: "project-1" });
    const preparedContent: ContentValue = {
      firstImageSrc: "mpp://media/asset-1",
      html: '<p><img src="mpp://media/asset-1" data-mpp-media-id="asset-1"></p>',
      text: "",
    };
    const prepareContentForSave = vi.fn().mockResolvedValue(preparedContent);
    const view = renderController("project-1");

    await act(async () => {
      await flushPromises();
      await flushPromises();
    });

    act(() => {
      useContentPageStore.setState({
        content: {
          firstImageSrc: "blob:http://localhost:3000/local-preview",
          html: '<img src="blob:http://localhost:3000/local-preview" data-mpp-local-media-id="local-1">',
          text: "",
        },
        selectedPlatforms: ["wechat"],
        title: "Post title",
      });
    });

    await act(async () => {
      (
        view.getController().header.onSave as
          | ((options: {
              prepareContentForSave: () => Promise<ContentValue>;
            }) => void)
          | undefined
      )?.({ prepareContentForSave });
      await flushPromises();
      await flushPromises();
    });

    expect(prepareContentForSave).toHaveBeenCalledOnce();
    expect(mocks.updateDashboardProject).toHaveBeenCalledWith("project-1", {
      cover_image_url: "mpp://media/asset-1",
      platforms: ["wechat"],
      source_content: preparedContent.html,
      summary: "",
      title: "Post title",
    });

    view.unmount();
  });

  it("blocks prepublish and publish actions while editor media is unsaved", async () => {
    mocks.getDashboardProject.mockResolvedValue({
      created_at: "2026-05-30T12:00:00.000Z",
      id: "project-1",
      publications: [
        { enabled: true, id: "pub-1", platform: "wechat", status: "draft" },
      ],
      role: "owner",
      source_content: "<p>Old body</p>",
      status: "ready",
      title: "Old title",
      updated_at: "2026-05-30T12:00:00.000Z",
      user_id: "user-1",
    });
    mocks.getProjectPublications.mockResolvedValue({
      items: [],
      project_id: "project-1",
    });
    const view = renderController("project-1");

    await act(async () => {
      await flushPromises();
      await flushPromises();
    });

    act(() => {
      useContentPageStore.setState({
        content: {
          firstImageSrc: "blob:http://localhost:3000/local-preview",
          html: '<p><img src="blob:http://localhost:3000/local-preview" data-mpp-local-media-id="local-1"></p>',
          text: "Rendered body",
        },
        prepublishDrafts: {
          wechat: {
            format: "html",
            raw: "<p>Rendered body</p>",
            syncedAt: "2026-05-30T12:00:00.000Z",
          },
        },
        selectedPlatforms: ["wechat"],
        title: "Post title",
      });
    });

    expect(view.getController().prepublish.canEdit).toBe(false);
    expect(view.getController().publishing.canPublish).toBe(false);
    expect(view.getController().publishing.canOpenXPostIntent).toBe(false);

    await act(async () => {
      await view.getController().prepublish.onSync(["wechat"]);
      view.getController().publishing.onPublish();
      view.getController().publishing.onOpenDouyinPublishSession();
      await flushPromises();
    });

    expect(mocks.updateDashboardProject).not.toHaveBeenCalled();
    expect(mocks.syncProjectPrepublish).not.toHaveBeenCalled();
    expect(mocks.saveDashboardProjectContent).not.toHaveBeenCalled();
    expect(mocks.publishProject).not.toHaveBeenCalled();
    expect(mocks.startDouyinPublishSession).not.toHaveBeenCalled();
    expect(mocks.toastError).toHaveBeenCalledWith(
      "project.savePendingMediaTitle",
      {
        description: "project.savePendingMediaDesc",
      },
    );

    view.unmount();
  });

  it("disables prepublish and publishing controls while saving", async () => {
    mocks.getDashboardProject.mockResolvedValue({
      created_at: "2026-05-30T12:00:00.000Z",
      id: "project-1",
      publications: [
        { enabled: true, id: "pub-1", platform: "wechat", status: "draft" },
      ],
      role: "owner",
      source_content: "<p>Old body</p>",
      status: "ready",
      title: "Old title",
      updated_at: "2026-05-30T12:00:00.000Z",
      user_id: "user-1",
    });
    mocks.getProjectPublications.mockResolvedValue({
      items: [],
      project_id: "project-1",
    });
    const view = renderController("project-1");

    await act(async () => {
      await flushPromises();
      await flushPromises();
    });

    act(() => {
      useContentPageStore.setState({
        content: {
          firstImageSrc: "",
          html: "<p>Rendered body</p>",
          text: "Rendered body",
        },
        isSaving: true,
        prepublishDrafts: {
          wechat: {
            format: "html",
            raw: "<p>Rendered body</p>",
            syncedAt: "2026-05-30T12:00:00.000Z",
          },
        },
        selectedPlatforms: ["wechat"],
        title: "Post title",
      });
    });

    expect(view.getController().prepublish.canEdit).toBe(false);
    expect(view.getController().publishing.canOpenXPostIntent).toBe(false);
    expect(view.getController().publishing.canPublish).toBe(false);
    expect(view.getController().publishing.canSelectPlatforms).toBe(false);

    view.unmount();
  });

  it("excludes Douyin from automatic publishing", async () => {
    mocks.getDashboardProject.mockResolvedValue({
      created_at: "2026-05-30T12:00:00.000Z",
      id: "project-1",
      publications: [
        { enabled: true, id: "pub-1", platform: "wechat", status: "draft" },
        { enabled: true, id: "pub-2", platform: "zhihu", status: "draft" },
      ],
      source_content: "<p>Rendered body</p>",
      role: "owner",
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
    mocks.saveDashboardProjectContent.mockResolvedValue({
      id: "project-1",
    });
    mocks.saveDashboardProjectPlatforms.mockResolvedValue({
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
          status: "succeeded",
          updated_at: "2026-05-30T12:00:00.000Z",
        },
      ],
      project_id: "project-1",
    });

    const view = renderController("project-1");

    await act(async () => {
      await flushPromises();
      await flushPromises();
    });

    expect(view.getController().header.projectRole).toBe("owner");

    act(() => {
      useContentPageStore.setState({
        content: {
          firstImageSrc: "",
          html: "<p>Rendered body</p>",
          text: "Rendered body",
        },
        prepublishDrafts: {
          douyin: {
            format: "text",
            raw: "Rendered body",
            syncedAt: "2026-05-30T12:00:00.000Z",
          },
          zhihu: {
            format: "markdown",
            raw: "Rendered body",
            syncedAt: "2026-05-30T12:00:00.000Z",
          },
        },
        selectedPlatforms: ["zhihu", "douyin"],
        title: "Post title",
      });
    });

    await act(async () => {
      view.getController().publishing.onPublish();
      await flushPromises();
    });

    expect(mocks.saveDashboardProjectContent).toHaveBeenCalledWith(
      "project-1",
      {
        cover_image_url: undefined,
        source_content: "<p>Rendered body</p>",
        summary: "Rendered body",
        title: "Post title",
      },
    );
    expect(mocks.saveDashboardProjectPlatforms).toHaveBeenCalledWith(
      "project-1",
      {
        platforms: ["zhihu"],
      },
    );
    expect(mocks.updateDashboardProject).not.toHaveBeenCalled();
    expect(mocks.publishProject).toHaveBeenCalledWith("project-1", "zhihu", {
      idempotencyKey: expect.stringMatching(/^project-1:zhihu:.+:zhihu$/),
    });
    expect(
      mocks.publishProject.mock.calls.some(
        ([, platform]) => platform === "douyin",
      ),
    ).toBe(false);
    expect(
      mocks.saveDashboardProjectContent.mock.invocationCallOrder[0],
    ).toBeLessThan(mocks.publishProject.mock.invocationCallOrder[0]);
    expect(
      mocks.saveDashboardProjectPlatforms.mock.invocationCallOrder[0],
    ).toBeLessThan(mocks.publishProject.mock.invocationCallOrder[0]);
    expect(mocks.waitForProjectPublications).toHaveBeenCalledWith("project-1", [
      "zhihu",
    ]);
    expect(mocks.toastSuccess).toHaveBeenCalledWith(
      "publish.editAndPublishSuccess",
      {
        description: "Published to Zhihu.",
      },
    );

    view.unmount();
  });

  it("opens Douyin manual publishing without adding X", async () => {
    mocks.createDashboardProject.mockResolvedValue({ id: "project-1" });
    mocks.saveDashboardProjectPlatforms.mockResolvedValue({
      id: "project-1",
    });
    mocks.syncProjectPrepublish.mockResolvedValue({
      items: [
        {
          adapted_content: {
            format: "text",
            source_revision: "2026-05-30T12:00:00.000Z",
            text: "Rendered body",
          },
          enabled: true,
          platform: "douyin",
          updated_at: "2026-05-30T12:00:00.000Z",
        },
      ],
      project_id: "project-1",
    });
    mocks.startDouyinPublishSession.mockResolvedValue({
      expires_at: "2026-05-30T12:30:00.000Z",
      session_id: "session-1",
      status: "active",
      stream_url: "/browser/session-1",
    });

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

    await act(async () => {
      view.getController().publishing.onOpenDouyinPublishSession();
      await flushPromises();
    });

    expect(mocks.createDashboardProject).toHaveBeenCalledWith({
      platforms: ["douyin"],
      source_content: "<p>Rendered body</p>",
      summary: "Rendered body",
      title: "Post title",
    });
    expect(mocks.saveDashboardProjectPlatforms).toHaveBeenCalledWith(
      "project-1",
      {
        platforms: ["douyin"],
      },
    );
    expect(mocks.syncProjectPrepublish).toHaveBeenCalledWith("project-1", {
      platforms: ["douyin"],
    });
    expect(mocks.publishProject).not.toHaveBeenCalledWith(
      "project-1",
      "x",
      expect.anything(),
    );
    expect(useContentPageStore.getState().selectedPlatforms).toEqual([
      "douyin",
    ]);
    expect(view.getController().publishing.douyinBrowserSession).toMatchObject({
      sessionId: "session-1",
      status: "active",
      streamURL: "/browser/session-1",
    });

    view.unmount();
  });
});

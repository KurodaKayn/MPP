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
  push: vi.fn(),
  refresh: vi.fn(),
  replace: vi.fn(),
  toastError: vi.fn(),
  toastSuccess: vi.fn(),
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
    mocks.push.mockReset();
    mocks.replace.mockReset();
    mocks.refresh.mockReset();
    mocks.toastError.mockReset();
    mocks.toastSuccess.mockReset();
    useContentPageStore.getState().resetForCreate();
  });

  it("syncs prepublish drafts with platform-specific formats", () => {
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

    act(() => {
      view.getController().syncPrepublish();
    });

    const state = useContentPageStore.getState();
    expect(state.prepublishDrafts.wechat).toMatchObject({
      format: "html",
      raw: "<p>Rendered body</p>",
    });
    expect(state.prepublishDrafts.zhihu).toMatchObject({
      format: "markdown",
      raw: "<p>Rendered body</p>",
    });
    expect(state.prepublishDrafts.x).toMatchObject({
      format: "text",
      raw: "<p>Rendered body</p>",
    });
    expect(state.isSyncingPrepublish).toBe(false);
    expect(mocks.toastSuccess).toHaveBeenCalledWith("已同步到预发布", {
      description: "暂未做格式转换，当前内容已复制到各平台草稿。",
    });

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
});

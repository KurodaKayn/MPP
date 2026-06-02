// @vitest-environment jsdom

import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";
import { ContentPrepublishPanel } from "./content-prepublish-panel";
import { ContentPublishBar } from "./content-publish-bar";
import { PlatformPreview } from "./platform-preview";

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

const commonTranslations: Record<string, string> = {
  "platforms.bilibili": "Bilibili",
  "platforms.wechat": "WeChat",
  "platforms.x": "X",
  "platforms.xiaohongshu": "Rednote",
  "platforms.zhihu": "Zhihu",
};

vi.mock("@/lib/i18n/client", () => ({
  useAppLocale: () => "en",
  useTranslation: () => ({
    t: (key: string) => commonTranslations[key] ?? key,
  }),
}));

vi.mock("next/image", () => ({
  default: ({
    height: _height,
    priority: _priority,
    width: _width,
    ...props
  }: React.ImgHTMLAttributes<HTMLImageElement> & {
    height?: number;
    priority?: boolean;
    width?: number;
  }) => createElement("img", props),
}));

function render(element: React.ReactElement) {
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);

  act(() => {
    root.render(element);
  });

  return {
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

describe("content platform labels", () => {
  it("renders localized labels in the automatic publish platform picker", () => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    const view = render(
      <ContentPublishBar
        canOpenXPostIntent={false}
        canPublish={false}
        canSelectPlatforms
        isOpeningXPostIntent={false}
        isPublishing={false}
        onOpenDouyinPublishSession={vi.fn()}
        onOpenXPostIntent={vi.fn()}
        onPublish={vi.fn()}
        onSelectedPlatformsChange={vi.fn()}
        selectedPlatforms={["wechat"]}
      />,
    );

    expect(view.text()).toContain("WeChat");
    expect(view.text()).not.toContain("platforms.wechat");

    view.unmount();
  });

  it("renders localized labels in the prepublish platform selector", () => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    const view = render(
      <ContentPrepublishPanel
        content={{
          firstImageSrc: "",
          html: "<p>Body</p>",
          text: "Body",
        }}
        drafts={{}}
        isSyncing={false}
        onDraftChange={vi.fn()}
        onSync={vi.fn()}
        title="Title"
      />,
    );

    expect(view.text()).toContain("WeChat");
    expect(view.text()).not.toContain("platforms.wechat");

    view.unmount();
  });

  it("renders localized labels in platform preview tabs", () => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    const view = render(
      <PlatformPreview
        title="Title"
        content={{
          firstImageSrc: "",
          html: "<p>Body</p>",
          text: "Body",
        }}
      />,
    );

    expect(view.text()).toContain("WeChat");
    expect(view.text()).not.toContain("platforms.wechat");

    view.unmount();
  });
});

// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ContentValue } from "@/lib/content/types";
import { useContentTipTapEditor } from "./use-content-tiptap-editor";

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

const mocks = vi.hoisted(() => ({
  completeMediaUpload: vi.fn(),
  createProjectMediaUpload: vi.fn(),
  resolveMediaAssets: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("@/lib/dashboard/api", () => ({
  completeMediaUpload: mocks.completeMediaUpload,
  createProjectMediaUpload: mocks.createProjectMediaUpload,
  resolveMediaAssets: mocks.resolveMediaAssets,
}));

vi.mock("@/lib/i18n/client", () => ({
  useAppLocale: () => "en",
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock("sonner", () => ({
  toast: {
    error: mocks.toastError,
  },
}));

type HookResult = ReturnType<typeof useContentTipTapEditor>;

function renderEditorHook(options?: {
  content?: ContentValue;
  onContentChange?: (content: ContentValue) => void;
  projectId?: string;
}) {
  let result: HookResult | undefined;
  const container = document.createElement("div");
  document.body.append(container);
  const root = createRoot(container);

  function Harness() {
    result = useContentTipTapEditor({
      content: options?.content ?? {
        firstImageSrc: "",
        html: "<p></p>",
        text: "",
      },
      onContentChange: options?.onContentChange ?? vi.fn(),
      projectId: options?.projectId,
    });
    return null;
  }

  act(() => {
    root.render(<Harness />);
  });

  return {
    getResult() {
      if (!result) {
        throw new Error("Editor hook did not render.");
      }
      return result;
    },
    unmount() {
      act(() => {
        root.unmount();
      });
      container.remove();
    },
  };
}

function changeEventForFiles(files: File[]) {
  return {
    target: {
      files,
      value: "selected",
    },
  } as unknown as React.ChangeEvent<HTMLInputElement>;
}

describe("useContentTipTapEditor", () => {
  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    mocks.completeMediaUpload.mockReset();
    mocks.createProjectMediaUpload.mockReset();
    mocks.resolveMediaAssets.mockReset();
    mocks.toastError.mockReset();

    vi.spyOn(URL, "createObjectURL").mockReturnValue(
      "blob:http://localhost:3000/local-preview",
    );
  });

  it("inserts a local preview without uploading when a project image is selected", () => {
    const view = renderEditorHook({ projectId: "project-1" });
    const file = new File(["image"], "draft.png", { type: "image/png" });

    act(() => {
      view.getResult().handleImageSelect(changeEventForFiles([file]));
    });

    const image = new DOMParser()
      .parseFromString(view.getResult().editor?.getHTML() ?? "", "text/html")
      .querySelector("img");

    expect(mocks.createProjectMediaUpload).not.toHaveBeenCalled();
    expect(image?.getAttribute("src")).toBe(
      "blob:http://localhost:3000/local-preview",
    );
    expect(image?.getAttribute("data-mpp-local-media-id")).toMatch(/^local-/);
    expect(image?.getAttribute("data-mpp-upload-status")).toBe("pending");

    view.unmount();
  });
});

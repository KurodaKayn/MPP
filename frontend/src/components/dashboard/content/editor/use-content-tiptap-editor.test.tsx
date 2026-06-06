// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
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

  it("uploads pending local images when preparing content for save", async () => {
    const view = renderEditorHook({ projectId: "project-1" });
    const file = new File(["image"], "draft.png", { type: "image/png" });
    mocks.createProjectMediaUpload.mockResolvedValue({
      asset_id: "asset-1",
      expires_at: "2026-06-06T12:10:00Z",
      headers: { "Content-Type": "image/png" },
      object_ref: "mpp://media/asset-1",
      upload_url: "https://r2.example/upload",
    });
    mocks.completeMediaUpload.mockResolvedValue({
      asset_id: "asset-1",
      object_ref: "mpp://media/asset-1",
      status: "ready",
    });
    mocks.resolveMediaAssets.mockResolvedValue({
      items: [
        {
          asset_id: "asset-1",
          expires_at: "2026-06-06T12:05:00Z",
          url: "https://r2.example/signed-preview",
        },
      ],
    });
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200 });
    vi.stubGlobal("fetch", fetchMock);

    act(() => {
      view.getResult().handleImageSelect(changeEventForFiles([file]));
    });

    let preparedContent: ContentValue | undefined;

    await act(async () => {
      preparedContent = await view.getResult().prepareContentForSave();
    });

    expect(mocks.createProjectMediaUpload).toHaveBeenCalledWith("project-1", {
      filename: "draft.png",
      mime_type: "image/png",
      size_bytes: 5,
      usage: "editor_image",
    });
    expect(fetchMock).toHaveBeenCalledWith("https://r2.example/upload", {
      body: file,
      headers: { "Content-Type": "image/png" },
      method: "PUT",
    });
    expect(mocks.completeMediaUpload).toHaveBeenCalledWith("asset-1");
    expect(preparedContent?.html).toContain("mpp://media/asset-1");
    expect(preparedContent?.html).toContain('data-mpp-media-id="asset-1"');
    expect(preparedContent?.html).not.toContain("blob:");
    expect(preparedContent?.html).not.toContain("data-mpp-local-media-id");
    expect(view.getResult().editor?.getHTML()).toContain(
      "https://r2.example/signed-preview",
    );

    view.unmount();
  });

  it("marks failed local uploads and retries them on the next save", async () => {
    const view = renderEditorHook({ projectId: "project-1" });
    const file = new File(["image"], "draft.png", { type: "image/png" });
    mocks.createProjectMediaUpload
      .mockResolvedValueOnce({
        asset_id: "asset-failed",
        expires_at: "2026-06-06T12:10:00Z",
        headers: { "Content-Type": "image/png" },
        object_ref: "mpp://media/asset-failed",
        upload_url: "https://r2.example/upload-failed",
      })
      .mockResolvedValueOnce({
        asset_id: "asset-2",
        expires_at: "2026-06-06T12:10:00Z",
        headers: { "Content-Type": "image/png" },
        object_ref: "mpp://media/asset-2",
        upload_url: "https://r2.example/upload-retry",
      });
    mocks.completeMediaUpload.mockResolvedValue({
      asset_id: "asset-2",
      object_ref: "mpp://media/asset-2",
      status: "ready",
    });
    mocks.resolveMediaAssets.mockResolvedValue({
      items: [
        {
          asset_id: "asset-2",
          expires_at: "2026-06-06T12:05:00Z",
          url: "https://r2.example/signed-retry",
        },
      ],
    });
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 500 })
      .mockResolvedValueOnce({ ok: true, status: 200 });
    vi.stubGlobal("fetch", fetchMock);

    act(() => {
      view.getResult().handleImageSelect(changeEventForFiles([file]));
    });

    await expect(
      act(async () => {
        await view.getResult().prepareContentForSave();
      }),
    ).rejects.toThrow("Upload failed (500)");

    expect(view.getResult().editor?.getHTML()).toContain(
      'data-mpp-upload-status="failed"',
    );
    expect(view.getResult().editor?.getHTML()).toContain(
      "blob:http://localhost:3000/local-preview",
    );
    expect(mocks.completeMediaUpload).not.toHaveBeenCalled();

    let preparedContent: ContentValue | undefined;

    await act(async () => {
      preparedContent = await view.getResult().prepareContentForSave();
    });

    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "https://r2.example/upload-retry",
      {
        body: file,
        headers: { "Content-Type": "image/png" },
        method: "PUT",
      },
    );
    expect(preparedContent?.html).toContain("mpp://media/asset-2");
    expect(preparedContent?.html).not.toContain("blob:");
    expect(preparedContent?.html).not.toContain("data-mpp-local-media-id");
    expect(preparedContent?.html).not.toContain("data-mpp-upload-status");
    expect(view.getResult().editor?.getHTML()).toContain(
      "https://r2.example/signed-retry",
    );

    view.unmount();
  });

  it("does not upload local images removed before save", async () => {
    const view = renderEditorHook({ projectId: "project-1" });
    const file = new File(["image"], "removed.png", { type: "image/png" });
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    act(() => {
      view.getResult().handleImageSelect(changeEventForFiles([file]));
      view.getResult().editor?.commands.clearContent();
    });

    let preparedContent: ContentValue | undefined;

    await act(async () => {
      preparedContent = await view.getResult().prepareContentForSave();
    });

    expect(mocks.createProjectMediaUpload).not.toHaveBeenCalled();
    expect(fetchMock).not.toHaveBeenCalled();
    expect(preparedContent?.html).not.toContain("blob:");
    expect(preparedContent?.html).not.toContain("data-mpp-local-media-id");

    view.unmount();
  });

  it("blocks save preparation when a pending image file is unavailable", async () => {
    const view = renderEditorHook({
      content: {
        firstImageSrc: "blob:http://localhost:3000/lost-preview",
        html: '<p><img src="blob:http://localhost:3000/lost-preview" data-mpp-local-media-id="local-lost" data-mpp-upload-status="pending"></p>',
        text: "",
      },
      projectId: "project-1",
    });

    await expect(
      act(async () => {
        await view.getResult().prepareContentForSave();
      }),
    ).rejects.toThrow("editor.imageUploadError");

    expect(mocks.createProjectMediaUpload).not.toHaveBeenCalled();
    expect(view.getResult().editor?.getHTML()).toContain("blob:");
    expect(view.getResult().editor?.getHTML()).toContain(
      'data-mpp-local-media-id="local-lost"',
    );

    view.unmount();
  });
});

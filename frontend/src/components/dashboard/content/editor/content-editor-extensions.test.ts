// @vitest-environment jsdom

import { Editor } from "@tiptap/react";
import { afterEach, describe, expect, it } from "vitest";
import { createContentEditorExtensions } from "./content-editor-extensions";

const editors: Editor[] = [];

function createTestEditor(content: string) {
  const editor = new Editor({
    content,
    extensions: createContentEditorExtensions({
      emptyEditorClassName: "empty",
      imageClassName: "image",
      linkClassName: "link",
    }),
  });

  editors.push(editor);
  return editor;
}

afterEach(() => {
  while (editors.length > 0) {
    editors.pop()?.destroy();
  }
});

describe("content editor extensions", () => {
  it("preserves local pending media attributes on image nodes", () => {
    const editor = createTestEditor(
      '<p><img src="blob:http://localhost:3000/preview" data-mpp-local-media-id="local-1" data-mpp-upload-status="pending" alt="draft"></p>',
    );

    const image = new DOMParser()
      .parseFromString(editor.getHTML(), "text/html")
      .querySelector("img");

    expect(image?.getAttribute("data-mpp-local-media-id")).toBe("local-1");
    expect(image?.getAttribute("data-mpp-upload-status")).toBe("pending");
  });
});

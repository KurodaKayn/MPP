import { yDocToProsemirrorJSON } from "@tiptap/y-tiptap";
import { describe, expect, it } from "vitest";

import { createProjectYDoc } from "./project-document.js";

describe("createProjectYDoc", () => {
  it("converts project HTML into the collaboration content field", () => {
    const document = createProjectYDoc(
      "<h2>Heading</h2><p>Hello <strong>team</strong></p>",
    );

    expect(yDocToProsemirrorJSON(document, "content")).toEqual({
      type: "doc",
      content: [
        {
          type: "heading",
          attrs: {
            level: 2,
          },
          content: [{ type: "text", text: "Heading" }],
        },
        {
          type: "paragraph",
          content: [
            { type: "text", text: "Hello " },
            {
              type: "text",
              marks: [{ type: "bold", attrs: {} }],
              text: "team",
            },
          ],
        },
      ],
    });

    document.destroy();
  });
});

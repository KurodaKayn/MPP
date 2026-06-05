import { describe, expect, it } from "vitest";

import {
  createProjectYDoc,
  projectYDocToHtml,
  projectYDocToProseMirrorJSON,
} from "./project-document.js";

describe("createProjectYDoc", () => {
  it("converts project HTML into the collaboration content field", () => {
    const document = createProjectYDoc(
      "<h2>Heading</h2><p>Hello <strong>team</strong></p>",
    );

    expect(projectYDocToProseMirrorJSON(document)).toEqual({
      type: "doc",
      content: [
        {
          type: "heading",
          attrs: {
            level: 2,
            textAlign: null,
          },
          content: [{ type: "text", text: "Heading" }],
        },
        {
          type: "paragraph",
          attrs: {
            textAlign: null,
          },
          content: [
            { type: "text", text: "Hello " },
            {
              type: "text",
              marks: [{ type: "bold" }],
              text: "team",
            },
          ],
        },
      ],
    });

    document.destroy();
  });

  it("converts project collaboration state back into HTML", () => {
    const document = createProjectYDoc(
      "<h2>Heading</h2><p>Hello <strong>team</strong></p>",
    );

    expect(projectYDocToHtml(document)).toBe(
      "<h2>Heading</h2><p>Hello <strong>team</strong></p>",
    );

    document.destroy();
  });
});

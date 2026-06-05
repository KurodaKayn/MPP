import { getSchema } from "@tiptap/core";
import { Blockquote } from "@tiptap/extension-blockquote";
import { Bold } from "@tiptap/extension-bold";
import { Code } from "@tiptap/extension-code";
import { CodeBlock } from "@tiptap/extension-code-block";
import { Document } from "@tiptap/extension-document";
import { HardBreak } from "@tiptap/extension-hard-break";
import { Heading } from "@tiptap/extension-heading";
import { HorizontalRule } from "@tiptap/extension-horizontal-rule";
import { Image } from "@tiptap/extension-image";
import { Italic } from "@tiptap/extension-italic";
import { Link } from "@tiptap/extension-link";
import { BulletList, ListItem, OrderedList } from "@tiptap/extension-list";
import { Paragraph } from "@tiptap/extension-paragraph";
import { Strike } from "@tiptap/extension-strike";
import { Text } from "@tiptap/extension-text";
import { TextAlign } from "@tiptap/extension-text-align";
import { Underline } from "@tiptap/extension-underline";
import { generateHTML, generateJSON } from "@tiptap/html";
import {
  prosemirrorJSONToYDoc,
  yXmlFragmentToProseMirrorRootNode,
} from "@tiptap/y-tiptap";

import type { Extensions } from "@tiptap/core";
import type { Doc as YDoc } from "yjs";

const projectContentExtensions: Extensions = [
  Bold,
  Blockquote,
  BulletList,
  Code,
  CodeBlock,
  Document,
  HardBreak,
  Heading.configure({
    levels: [1, 2, 3],
  }),
  HorizontalRule,
  Italic,
  ListItem,
  OrderedList,
  Paragraph,
  Strike,
  Text,
  Underline,
  Link.configure({
    autolink: true,
    defaultProtocol: "https",
    linkOnPaste: true,
    openOnClick: false,
    HTMLAttributes: {
      rel: "noopener noreferrer",
      target: "_blank",
    },
  }),
  Image.configure({
    allowBase64: true,
    inline: false,
    resize: {
      enabled: true,
      directions: ["left", "right"],
      minWidth: 160,
      alwaysPreserveAspectRatio: true,
    },
  }),
  TextAlign.configure({
    types: ["heading", "paragraph"],
  }),
];

const projectContentSchema = getSchema(projectContentExtensions);

export function createProjectYDoc(sourceContent: string) {
  const content = generateJSON(sourceContent, projectContentExtensions);
  return prosemirrorJSONToYDoc(projectContentSchema, content, "content");
}

export function projectYDocToProseMirrorJSON(document: YDoc) {
  return yXmlFragmentToProseMirrorRootNode(
    document.getXmlFragment("content"),
    projectContentSchema,
  ).toJSON();
}

export function projectYDocToHtml(document: YDoc) {
  return generateHTML(
    projectYDocToProseMirrorJSON(document),
    projectContentExtensions,
  );
}

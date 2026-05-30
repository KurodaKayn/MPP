import ImageExtension from "@tiptap/extension-image";
import LinkExtension from "@tiptap/extension-link";
import Placeholder from "@tiptap/extension-placeholder";
import TextAlign from "@tiptap/extension-text-align";
import StarterKit from "@tiptap/starter-kit";

type ContentEditorExtensionOptions = {
  emptyEditorClassName: string;
  imageClassName: string;
  linkClassName: string;
};

export function createContentEditorExtensions({
  emptyEditorClassName,
  imageClassName,
  linkClassName,
}: ContentEditorExtensionOptions) {
  return [
    StarterKit.configure({
      heading: {
        levels: [1, 2, 3],
      },
      link: false,
    }),
    LinkExtension.configure({
      autolink: true,
      defaultProtocol: "https",
      linkOnPaste: true,
      openOnClick: false,
      HTMLAttributes: {
        class: linkClassName,
        rel: "noopener noreferrer",
        target: "_blank",
      },
    }),
    ImageExtension.configure({
      allowBase64: true,
      inline: false,
      HTMLAttributes: {
        class: imageClassName,
      },
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
    Placeholder.configure({
      placeholder: "在这里开始创作...",
      emptyEditorClass: emptyEditorClassName,
    }),
  ];
}

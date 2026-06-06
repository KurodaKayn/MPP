import ImageExtension from "@tiptap/extension-image";
import LinkExtension from "@tiptap/extension-link";
import Placeholder from "@tiptap/extension-placeholder";
import TextAlign from "@tiptap/extension-text-align";
import { Markdown } from "@tiptap/markdown";
import StarterKit from "@tiptap/starter-kit";

type ContentEditorExtensionOptions = {
  emptyEditorClassName: string;
  enableUndoRedo?: boolean;
  imageClassName: string;
  linkClassName: string;
  placeholder?: string;
};

const MppImageExtension = ImageExtension.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      mppMediaId: {
        default: null,
        parseHTML: (element: HTMLElement) =>
          element.getAttribute("data-mpp-media-id"),
        renderHTML: (attributes: Record<string, unknown>) => {
          const mediaId = attributes.mppMediaId;

          if (typeof mediaId !== "string" || !mediaId.trim()) {
            return {};
          }

          return {
            "data-mpp-media-id": mediaId,
          };
        },
      },
      mppLocalMediaId: {
        default: null,
        parseHTML: (element: HTMLElement) =>
          element.getAttribute("data-mpp-local-media-id"),
        renderHTML: (attributes: Record<string, unknown>) => {
          const localMediaId = attributes.mppLocalMediaId;

          if (typeof localMediaId !== "string" || !localMediaId.trim()) {
            return {};
          }

          return {
            "data-mpp-local-media-id": localMediaId,
          };
        },
      },
      mppUploadStatus: {
        default: null,
        parseHTML: (element: HTMLElement) =>
          element.getAttribute("data-mpp-upload-status"),
        renderHTML: (attributes: Record<string, unknown>) => {
          const uploadStatus = attributes.mppUploadStatus;

          if (typeof uploadStatus !== "string" || !uploadStatus.trim()) {
            return {};
          }

          return {
            "data-mpp-upload-status": uploadStatus,
          };
        },
      },
    };
  },
});

export function createContentEditorExtensions({
  emptyEditorClassName,
  enableUndoRedo = true,
  imageClassName,
  linkClassName,
  placeholder = "Start writing...",
}: ContentEditorExtensionOptions) {
  return [
    StarterKit.configure({
      heading: {
        levels: [1, 2, 3],
      },
      link: false,
      undoRedo: enableUndoRedo ? undefined : false,
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
    MppImageExtension.configure({
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
      placeholder,
      emptyEditorClass: emptyEditorClassName,
    }),
    Markdown,
  ];
}

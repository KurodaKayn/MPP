import { useEditor, type Editor } from "@tiptap/react";
import { useEffect, useMemo, type ChangeEvent } from "react";
import { toast } from "sonner";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";

import { createContentEditorExtensions } from "@/components/dashboard/content/editor/content-editor-extensions";
import {
  MAX_INLINE_IMAGE_SIZE,
  contentValueFromHtml,
  normalizeStoredHtml,
  normalizeUrl,
  sanitizeClipboardHtml,
} from "@/components/dashboard/content/editor/content-editor-utils";
import type { ContentValue } from "@/lib/content/types";
import styles from "./content-editor.module.css";

type UseContentTipTapEditorOptions = {
  content: ContentValue;
  editable?: boolean;
  onContentChange: (content: ContentValue) => void;
};

export function useContentTipTapEditor({
  content,
  editable = true,
  onContentChange,
}: UseContentTipTapEditorOptions) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");

  const extensions = useMemo(
    () =>
      createContentEditorExtensions({
        emptyEditorClassName: styles.emptyEditor,
        imageClassName: styles.image,
        linkClassName: styles.link,
        placeholder: t("editor.placeholder"),
      }),
    [],
  );
  const editor = useEditor({
    extensions,
    content: normalizeStoredHtml(content.html),
    editable,
    editorProps: {
      attributes: {
        "aria-label": t("editor.ariaLabel"),
        class: styles.prose,
      },
      handleDrop: (_view, event) => {
        if (!editable) {
          return false;
        }

        const files = getImageFiles(event.dataTransfer?.files);

        if (files.length === 0) {
          return false;
        }

        event.preventDefault();
        return insertImageFiles(files);
      },
      handlePaste: (_view, event) => {
        if (!editable) {
          return false;
        }

        const files = getImageFiles(event.clipboardData?.files);

        if (files.length > 0) {
          event.preventDefault();
          return insertImageFiles(files);
        }

        const html = event.clipboardData?.getData("text/html");

        if (!html || !editor) {
          return false;
        }

        event.preventDefault();
        editor.chain().focus().insertContent(sanitizeClipboardHtml(html)).run();
        return true;
      },
    },
    immediatelyRender: false,
    shouldRerenderOnTransaction: true,
    onUpdate: ({ editor }) => {
      onContentChange(contentValueFromHtml(editor.getHTML()));
    },
  });

  useEffect(() => {
    if (!editor || editor.isDestroyed) {
      return;
    }

    editor.setEditable(editable);
  }, [editable, editor]);

  useEffect(() => {
    if (!editor || editor.isDestroyed) {
      return;
    }

    const nextHtml = normalizeStoredHtml(content.html);

    if (nextHtml === editor.getHTML()) {
      return;
    }

    editor.commands.setContent(nextHtml, { emitUpdate: false });
  }, [content.html, editor]);

  function insertImageFiles(files: File[]) {
    if (!editable || !editor || editor.isDestroyed) {
      return false;
    }

    const imageFiles = files.filter((file) => file.type.startsWith("image/"));

    if (imageFiles.length === 0) {
      toast.error(t("editor.selectImageError"));
      return false;
    }

    const oversizeFile = imageFiles.find(
      (file) => file.size > MAX_INLINE_IMAGE_SIZE,
    );

    if (oversizeFile) {
      toast.error(t("editor.imageSizeError"));
      return false;
    }

    imageFiles.forEach((file) => {
      const reader = new FileReader();

      reader.onload = () => {
        if (typeof reader.result !== "string") {
          return;
        }

        editor
          .chain()
          .focus()
          .setImage({
            alt: file.name || t("toolbar.insertImage"),
            src: reader.result,
          })
          .run();
      };

      reader.readAsDataURL(file);
    });

    return true;
  }

  function handleImageSelect(event: ChangeEvent<HTMLInputElement>) {
    const files = getImageFiles(event.target.files);
    event.target.value = "";

    if (files.length === 0) {
      return;
    }

    insertImageFiles(files);
  }

  function setLink() {
    if (!editable || !editor) {
      return;
    }

    const currentHref = editor.getAttributes("link").href;
    const href = window.prompt(
      t("editor.linkPrompt"),
      typeof currentHref === "string" ? currentHref : "",
    );

    if (href === null) {
      return;
    }

    if (href.trim() === "") {
      editor.chain().focus().extendMarkRange("link").unsetLink().run();
      return;
    }

    const safeHref = normalizeUrl(href);

    if (!safeHref) {
      toast.error(t("editor.linkError"));
      return;
    }

    editor
      .chain()
      .focus()
      .extendMarkRange("link")
      .setLink({ href: safeHref })
      .run();
  }

  return {
    editor,
    handleImageSelect,
    imageCount: countImages(editor),
    setLink,
  };
}

function getImageFiles(files: FileList | null | undefined) {
  return Array.from(files ?? []).filter((file) =>
    file.type.startsWith("image/"),
  );
}

function countImages(editor: Editor | null) {
  let imageCount = 0;

  editor?.state.doc.descendants((node) => {
    if (node.type.name === "image") {
      imageCount += 1;
    }
  });

  return imageCount;
}

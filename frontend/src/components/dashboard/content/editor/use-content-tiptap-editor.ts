import Collaboration from "@tiptap/extension-collaboration";
import CollaborationCaret from "@tiptap/extension-collaboration-caret";
import type { HocuspocusProvider } from "@hocuspocus/provider";
import { useEditor, type Editor } from "@tiptap/react";
import { useEffect, useMemo, type ChangeEvent } from "react";
import { toast } from "sonner";
import type * as Y from "yjs";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";

import { createContentEditorExtensions } from "@/components/dashboard/content/editor/content-editor-extensions";
import {
  MAX_INLINE_IMAGE_SIZE,
  contentValueFromHtml,
  normalizeStoredHtml,
  normalizeUrl,
  sanitizeClipboardHtml,
} from "@/components/dashboard/content/editor/content-editor-utils";
import {
  renderCollabCursor,
  renderCollabSelection,
  type CollabUserProfile,
} from "@/features/collab-editor/collab-provider";
import type { ContentValue } from "@/lib/content/types";
import styles from "./content-editor.module.css";

export type ContentEditorCollaborationProvider = {
  canEdit: boolean;
  provider: HocuspocusProvider | null;
  user: CollabUserProfile | null;
  ydoc: Y.Doc | null;
};

type UseContentTipTapEditorOptions = {
  collaboration?: ContentEditorCollaborationProvider;
  content: ContentValue;
  editable?: boolean;
  onContentChange: (content: ContentValue) => void;
};

export function useContentTipTapEditor({
  collaboration,
  content,
  editable = true,
  onContentChange,
}: UseContentTipTapEditorOptions) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");
  const collaborationProvider = collaboration?.provider ?? null;
  const collaborationUser = collaboration?.user ?? null;
  const collaborationYdoc = collaboration?.ydoc ?? null;
  const isCollaborationReady = Boolean(
    collaborationProvider && collaborationUser && collaborationYdoc,
  );
  const canEdit = editable && (!collaboration || collaboration.canEdit);

  const extensions = useMemo(() => {
    const baseExtensions = createContentEditorExtensions({
      emptyEditorClassName: styles.emptyEditor,
      enableUndoRedo: !isCollaborationReady,
      imageClassName: styles.image,
      linkClassName: styles.link,
      placeholder: t("editor.placeholder"),
    });

    if (!collaborationProvider || !collaborationUser || !collaborationYdoc) {
      return baseExtensions;
    }

    return [
      ...baseExtensions,
      Collaboration.configure({
        document: collaborationYdoc,
        field: "content",
      }),
      CollaborationCaret.configure({
        provider: collaborationProvider,
        render: renderCollabCursor,
        selectionRender: renderCollabSelection,
        user: collaborationUser,
      }),
    ];
  }, [
    collaborationProvider,
    collaborationUser,
    collaborationYdoc,
    isCollaborationReady,
    t,
  ]);
  const editor = useEditor(
    {
      extensions,
      content: isCollaborationReady
        ? undefined
        : normalizeStoredHtml(content.html),
      editable: canEdit,
      editorProps: {
        attributes: {
          "aria-label": t("editor.ariaLabel"),
          class: styles.prose,
        },
        handleDrop: (_view, event) => {
          if (!canEdit) {
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
          if (!canEdit) {
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
          editor
            .chain()
            .focus()
            .insertContent(sanitizeClipboardHtml(html))
            .run();
          return true;
        },
      },
      immediatelyRender: false,
      shouldRerenderOnTransaction: true,
      onUpdate: ({ editor }) => {
        onContentChange(contentValueFromHtml(editor.getHTML()));
      },
    },
    [
      collaborationProvider,
      collaborationUser?.color,
      collaborationUser?.name,
      collaborationUser?.role,
      collaborationYdoc,
      isCollaborationReady,
    ],
  );

  useEffect(() => {
    if (!editor || editor.isDestroyed) {
      return;
    }

    editor.setEditable(canEdit);
  }, [canEdit, editor]);

  useEffect(() => {
    if (isCollaborationReady || !editor || editor.isDestroyed) {
      return;
    }

    const nextHtml = normalizeStoredHtml(content.html);

    if (nextHtml === editor.getHTML()) {
      return;
    }

    editor.commands.setContent(nextHtml, { emitUpdate: false });
  }, [content.html, editor, isCollaborationReady]);

  function insertImageFiles(files: File[]) {
    if (!canEdit || !editor || editor.isDestroyed) {
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
    if (!canEdit || !editor) {
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

import Collaboration from "@tiptap/extension-collaboration";
import CollaborationCaret from "@tiptap/extension-collaboration-caret";
import type { HocuspocusProvider } from "@hocuspocus/provider";
import { useEditor, type Editor } from "@tiptap/react";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  type ChangeEvent,
} from "react";
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
  collectMediaAssetIds,
  createLocalMediaId,
  hydrateMediaAssetRefs,
  serializeMediaAssetRefs,
} from "@/components/dashboard/content/editor/content-editor-media";
import {
  renderCollabCursor,
  renderCollabSelection,
  type CollabUserProfile,
} from "@/features/collab-editor/collab-provider";
import type { ContentValue } from "@/lib/content/types";
import { resolveMediaAssets } from "@/lib/dashboard/api";
import styles from "./content-editor.module.css";

const COLLAB_CONTENT_SYNC_DEBOUNCE_MS = 250;

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
  projectId?: string;
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
  const lastEmittedHtmlRef = useRef("");
  const onContentChangeRef = useRef(onContentChange);
  const pendingLocalMediaRef = useRef<
    Map<string, { file: File; previewURL: string }>
  >(new Map());
  const pendingContentUpdateRef = useRef<ReturnType<
    typeof globalThis.setTimeout
  > | null>(null);

  useEffect(() => {
    onContentChangeRef.current = onContentChange;
  }, [onContentChange]);

  const emitContentChange = useCallback((activeEditor: Editor) => {
    const html = serializeMediaAssetRefs(activeEditor.getHTML());

    if (html === lastEmittedHtmlRef.current) {
      return;
    }

    lastEmittedHtmlRef.current = html;
    onContentChangeRef.current(contentValueFromHtml(html));
  }, []);

  const scheduleContentChange = useCallback(
    (activeEditor: Editor) => {
      if (!isCollaborationReady) {
        emitContentChange(activeEditor);
        return;
      }

      if (pendingContentUpdateRef.current !== null) {
        return;
      }

      pendingContentUpdateRef.current = globalThis.setTimeout(() => {
        pendingContentUpdateRef.current = null;

        if (!activeEditor.isDestroyed) {
          emitContentChange(activeEditor);
        }
      }, COLLAB_CONTENT_SYNC_DEBOUNCE_MS);
    },
    [emitContentChange, isCollaborationReady],
  );

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
        scheduleContentChange(editor);
      },
    },
    [
      collaborationProvider,
      collaborationUser?.color,
      collaborationUser?.name,
      collaborationUser?.role,
      collaborationYdoc,
      isCollaborationReady,
      scheduleContentChange,
    ],
  );

  useEffect(() => {
    if (!editor || editor.isDestroyed) {
      return;
    }

    editor.setEditable(canEdit);
  }, [canEdit, editor]);

  useEffect(() => {
    return () => {
      if (pendingContentUpdateRef.current !== null) {
        globalThis.clearTimeout(pendingContentUpdateRef.current);
        pendingContentUpdateRef.current = null;
      }
      pendingLocalMediaRef.current.forEach((media) => {
        URL.revokeObjectURL(media.previewURL);
      });
      pendingLocalMediaRef.current.clear();
    };
  }, []);

  useEffect(() => {
    if (isCollaborationReady || !editor || editor.isDestroyed) {
      return;
    }

    const nextHtml = normalizeStoredHtml(content.html);

    if (nextHtml === serializeMediaAssetRefs(editor.getHTML())) {
      return;
    }

    lastEmittedHtmlRef.current = nextHtml;
    editor.commands.setContent(nextHtml, { emitUpdate: false });
  }, [content.html, editor, isCollaborationReady]);

  useEffect(() => {
    if (isCollaborationReady || !editor || editor.isDestroyed) {
      return;
    }

    const activeEditor = editor;
    const assetIds = collectMediaAssetIds(content.html);

    if (assetIds.length === 0) {
      return;
    }

    let cancelled = false;

    async function hydrateEditorMedia() {
      try {
        const resolved = await resolveMediaAssets(assetIds);
        if (cancelled || activeEditor.isDestroyed) {
          return;
        }

        const nextHtml = normalizeStoredHtml(
          hydrateMediaAssetRefs(content.html, resolved.items),
        );

        if (nextHtml === activeEditor.getHTML()) {
          return;
        }

        lastEmittedHtmlRef.current = serializeMediaAssetRefs(nextHtml);
        activeEditor.commands.setContent(nextHtml, { emitUpdate: false });
      } catch (requestError) {
        if (!cancelled) {
          toast.error(t("editor.imagePreviewError"), {
            description:
              requestError instanceof Error
                ? requestError.message
                : t("common.retryLater"),
          });
        }
      }
    }

    void hydrateEditorMedia();

    return () => {
      cancelled = true;
    };
  }, [content.html, editor, isCollaborationReady, t]);

  function insertImageFiles(files: File[]) {
    if (!canEdit || !editor || editor.isDestroyed) {
      return false;
    }

    const activeEditor = editor;
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
      insertLocalImageFile(file, activeEditor);
    });

    return true;
  }

  function insertLocalImageFile(file: File, activeEditor: Editor) {
    if (activeEditor.isDestroyed) {
      return;
    }

    const localMediaId = createLocalMediaId();
    const previewURL = URL.createObjectURL(file);
    pendingLocalMediaRef.current.set(localMediaId, { file, previewURL });

    activeEditor
      .chain()
      .focus()
      .insertContent({
        attrs: {
          alt: file.name || t("toolbar.insertImage"),
          mppLocalMediaId: localMediaId,
          mppUploadStatus: "pending",
          src: previewURL,
        },
        type: "image",
      })
      .run();
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

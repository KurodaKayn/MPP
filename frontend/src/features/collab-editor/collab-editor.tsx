"use client";

import Collaboration from "@tiptap/extension-collaboration";
import CollaborationCaret from "@tiptap/extension-collaboration-caret";
import { EditorContent, useEditor, type Editor } from "@tiptap/react";
import type { HocuspocusProvider } from "@hocuspocus/provider";
import type * as Y from "yjs";
import {
  FileText,
  Loader2,
  Plus,
  RefreshCw,
  Save,
  UsersRound,
} from "lucide-react";
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type FormEvent,
} from "react";
import { toast } from "sonner";
import { useAuth } from "@/components/auth/auth-provider";
import { createContentEditorExtensions } from "@/components/dashboard/content/editor/content-editor-extensions";
import {
  normalizeUrl,
  sanitizeClipboardHtml,
} from "@/components/dashboard/content/editor/content-editor-utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import { cn } from "@/lib/utils";
import {
  renderCollabCursor,
  renderCollabSelection,
  type CollabUserProfile,
} from "./collab-provider";
import styles from "./collab-editor.module.css";
import { CollabEditorToolbar } from "./toolbar";
import { useCollabConnection, useCollabDocuments } from "./use-collab-document";

export function CollabEditorPage() {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const { session } = useAuth();
  const [newTitle, setNewTitle] = useState("");

  const showError = useCallback(
    (message: string) => {
      toast.error(t("collab.toast.requestFailed"), {
        description: message,
      });
    },
    [t],
  );

  const collabDocuments = useCollabDocuments({
    defaultTitle: t("collab.documents.defaultTitle"),
    fallbackError: t("collab.toast.retryLater"),
    onError: showError,
  });
  const connection = useCollabConnection({
    document: collabDocuments.selectedDocument,
    userName: session?.username ?? t("collab.userFallback"),
  });
  const editor = useCollabTipTapEditor({
    canEdit: connection.canEdit,
    provider: connection.provider,
    user: connection.user,
    ydoc: connection.ydoc,
  });
  const [titleDraft, setTitleDraft] = useState("");

  useEffect(() => {
    setTitleDraft(collabDocuments.selectedDocument?.title ?? "");
  }, [
    collabDocuments.selectedDocument?.id,
    collabDocuments.selectedDocument?.title,
  ]);

  async function handleCreateDocument(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const created = await collabDocuments.createDocument(newTitle);

    if (created) {
      setNewTitle("");
    }
  }

  async function saveTitle() {
    const selectedDocument = collabDocuments.selectedDocument;

    if (!selectedDocument) {
      return;
    }

    const renamed = await collabDocuments.renameDocument(
      selectedDocument,
      titleDraft,
    );

    if (!renamed) {
      setTitleDraft(selectedDocument.title);
    }
  }

  return (
    <div className="flex flex-col gap-6 pb-4">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="mb-2 flex items-center gap-2">
            <Badge variant="outline" className="gap-1">
              <UsersRound className="size-3" />
              {t("collab.header.badge")}
            </Badge>
          </div>
          <h2 className="text-3xl font-bold tracking-tight">
            {t("collab.header.title")}
          </h2>
          <p className="text-muted-foreground">
            {t("collab.header.description")}
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          onClick={() => void collabDocuments.loadDocuments()}
          disabled={collabDocuments.isLoading}
        >
          <RefreshCw
            className={cn(
              "size-4",
              collabDocuments.isLoading && "animate-spin",
            )}
          />
          {t("collab.documents.refresh")}
        </Button>
      </div>

      <div className="grid gap-4 lg:grid-cols-[20rem_minmax(0,1fr)]">
        <Card className="h-fit">
          <CardHeader>
            <CardTitle>{t("collab.documents.title")}</CardTitle>
            <CardDescription>
              {t("collab.documents.description")}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <form className="flex gap-2" onSubmit={handleCreateDocument}>
              <Input
                value={newTitle}
                placeholder={t("collab.documents.newPlaceholder")}
                onChange={(event) => setNewTitle(event.target.value)}
              />
              <Button type="submit" disabled={collabDocuments.isCreating}>
                {collabDocuments.isCreating ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <Plus className="size-4" />
                )}
                <span className="sr-only">{t("collab.documents.create")}</span>
              </Button>
            </form>

            <DocumentList
              documents={collabDocuments.documents}
              isLoading={collabDocuments.isLoading}
              selectedDocumentId={collabDocuments.selectedDocumentId}
              onSelect={collabDocuments.selectDocument}
            />
          </CardContent>
        </Card>

        <Card className="min-w-0">
          {collabDocuments.selectedDocument ? (
            <>
              <CardHeader className="gap-3">
                <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
                  <div className="min-w-0 flex-1">
                    <Input
                      value={titleDraft}
                      aria-label={t("collab.editor.titleLabel")}
                      className="h-auto border-0 px-0 py-0 text-2xl font-semibold shadow-none focus-visible:ring-0 sm:text-3xl"
                      onBlur={() => void saveTitle()}
                      onChange={(event) => setTitleDraft(event.target.value)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter") {
                          event.currentTarget.blur();
                        }
                      }}
                    />
                    <CardDescription>
                      {t("collab.editor.updatedAt", {
                        date: formatDate(
                          collabDocuments.selectedDocument.updated_at,
                          locale,
                        ),
                      })}
                    </CardDescription>
                  </div>
                  {collabDocuments.isRenaming ? (
                    <Badge variant="outline" className="gap-1">
                      <Save className="size-3" />
                      {t("collab.editor.savingTitle")}
                    </Badge>
                  ) : null}
                </div>
              </CardHeader>
              <CardContent>
                {connection.error ? (
                  <div className="mb-4 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
                    {connection.error}
                  </div>
                ) : null}
                {!connection.canEdit && connection.role ? (
                  <div className="mb-4 rounded-lg border bg-muted/40 p-3 text-sm text-muted-foreground">
                    {t("collab.editor.readonly")}
                  </div>
                ) : null}
                <div className={styles.editorShell}>
                  <CollabEditorToolbar
                    canEdit={connection.canEdit}
                    editor={editor}
                    onSetLink={() => setLink(editor, connection.canEdit, t)}
                    onlineUsers={connection.onlineUsers}
                    role={connection.role}
                    status={connection.status}
                    unsyncedChanges={connection.unsyncedChanges}
                  />
                  <div className={styles.editorArea}>
                    {connection.isConnecting ? (
                      <EditorSkeleton />
                    ) : (
                      <EditorContent editor={editor} />
                    )}
                  </div>
                </div>
              </CardContent>
            </>
          ) : (
            <CardContent>
              <div className="flex min-h-[520px] flex-col items-center justify-center rounded-xl border border-dashed text-center">
                <FileText className="mb-3 size-10 text-muted-foreground" />
                <h3 className="font-semibold">{t("collab.empty.title")}</h3>
                <p className="mt-1 max-w-sm text-sm text-muted-foreground">
                  {collabDocuments.error || t("collab.empty.description")}
                </p>
              </div>
            </CardContent>
          )}
        </Card>
      </div>
    </div>
  );
}

function useCollabTipTapEditor({
  canEdit,
  provider,
  user,
  ydoc,
}: {
  canEdit: boolean;
  provider: HocuspocusProvider | null;
  user: CollabUserProfile | null;
  ydoc: Y.Doc | null;
}) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const extensions = useMemo(() => {
    const baseExtensions = createContentEditorExtensions({
      emptyEditorClassName: styles.emptyEditor,
      enableUndoRedo: !provider,
      imageClassName: styles.image,
      linkClassName: styles.link,
      placeholder: t("collab.editor.placeholder"),
    });

    if (!ydoc || !provider || !user) {
      return baseExtensions;
    }

    return [
      ...baseExtensions,
      Collaboration.configure({
        document: ydoc,
        field: "content",
      }),
      CollaborationCaret.configure({
        provider,
        render: renderCollabCursor,
        selectionRender: renderCollabSelection,
        user,
      }),
    ];
  }, [provider, t, user, ydoc]);

  const editor = useEditor(
    {
      editable: canEdit,
      editorProps: {
        attributes: {
          "aria-label": t("collab.editor.ariaLabel"),
          class: styles.prose,
        },
        handlePaste: (_view, event) => {
          if (!canEdit) {
            return false;
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
      extensions,
      immediatelyRender: false,
      shouldRerenderOnTransaction: true,
    },
    [provider, ydoc, user?.name, user?.color],
  );

  useEffect(() => {
    editor?.setEditable(canEdit);
  }, [canEdit, editor]);

  return editor;
}

function DocumentList({
  documents,
  isLoading,
  onSelect,
  selectedDocumentId,
}: {
  documents: { id: string; title: string; updated_at: string }[];
  isLoading: boolean;
  onSelect: (documentId: string) => void;
  selectedDocumentId: string | null;
}) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");

  if (isLoading) {
    return (
      <div className="space-y-2">
        {Array.from({ length: 5 }).map((_, index) => (
          <Skeleton key={index} className="h-14 w-full" />
        ))}
      </div>
    );
  }

  if (documents.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
        {t("collab.documents.empty")}
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {documents.map((document) => (
        <button
          key={document.id}
          type="button"
          className={cn(
            "w-full rounded-lg border p-3 text-left transition hover:bg-muted/50",
            selectedDocumentId === document.id &&
              "border-primary bg-primary/5 ring-1 ring-primary/20",
          )}
          onClick={() => onSelect(document.id)}
        >
          <div className="truncate font-medium">{document.title}</div>
          <div className="mt-1 text-xs text-muted-foreground">
            {formatDate(document.updated_at, locale)}
          </div>
        </button>
      ))}
    </div>
  );
}

function EditorSkeleton() {
  return (
    <div className="space-y-4">
      <Skeleton className="h-5 w-11/12" />
      <Skeleton className="h-5 w-8/12" />
      <Skeleton className="h-5 w-10/12" />
      <Skeleton className="h-40 w-full" />
    </div>
  );
}

function setLink(
  editor: Editor | null,
  canEdit: boolean,
  t: (key: string) => string,
) {
  if (!editor || !canEdit) {
    return;
  }

  const currentHref = editor.getAttributes("link").href;
  const href = window.prompt(
    t("collab.editor.linkPrompt"),
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
    toast.error(t("collab.editor.linkError"));
    return;
  }

  editor
    .chain()
    .focus()
    .extendMarkRange("link")
    .setLink({ href: safeHref })
    .run();
}

function formatDate(value: string, locale: string) {
  return new Intl.DateTimeFormat(locale, {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "short",
  }).format(new Date(value));
}

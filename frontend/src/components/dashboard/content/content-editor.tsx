"use client";

import { UsersRound } from "lucide-react";
import { useRef } from "react";

import { AIEditAssistant } from "@/components/dashboard/content/ai/ai-edit-assistant";
import { getCurrentBlockLabel } from "@/components/dashboard/content/editor/content-editor-block-menu";
import {
  ContentEditorBody,
  ContentEditorTitle,
} from "@/components/dashboard/content/editor/content-editor-document";
import { ContentEditorToolbar } from "@/components/dashboard/content/editor/content-editor-toolbar";
import { contentValueFromHtml } from "@/components/dashboard/content/editor/content-editor-utils";
import {
  useContentTipTapEditor,
  type ContentEditorCollaborationProvider,
} from "@/components/dashboard/content/editor/use-content-tiptap-editor";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { TooltipProvider } from "@/components/ui/tooltip";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import { streamAIContentEdit } from "@/lib/dashboard/api";
import type { CollabDocumentRole } from "@/lib/dashboard/api";
import type { ContentValue } from "@/lib/content/types";
import type {
  CollabConnectionStatus,
  CollabUserProfile,
} from "@/features/collab-editor/collab-provider";
import { cn } from "@/lib/utils";

type ContentEditorProps = {
  canEdit?: boolean;
  collaboration?: ContentEditorCollaboration;
  title: string;
  content: ContentValue;
  onTitleChange: (title: string) => void;
  onContentChange: (content: ContentValue) => void;
  viewSwitcher?: React.ReactNode;
};

export type ContentEditorCollaboration = ContentEditorCollaborationProvider & {
  error: string;
  onlineUsers: CollabUserProfile[];
  role: CollabDocumentRole | null;
  status: CollabConnectionStatus;
  unsyncedChanges: number;
};

export function ContentEditor({
  canEdit = true,
  collaboration,
  title,
  content,
  onTitleChange,
  onContentChange,
  viewSwitcher,
}: ContentEditorProps) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");
  const { editor, handleImageSelect, imageCount, setLink } =
    useContentTipTapEditor({
      collaboration,
      content,
      editable: canEdit,
      onContentChange,
    });
  const canEditContent = canEdit && (!collaboration || collaboration.canEdit);
  const blockLabel = getCurrentBlockLabel(editor, t);
  const aiSource = editor?.getMarkdown?.() || content.text || content.html;

  const applyAIProposal = (proposal: string) => {
    if (!editor || editor.isDestroyed) {
      return;
    }

    editor.commands.setContent(proposal, { contentType: "markdown" });
    onContentChange(contentValueFromHtml(editor.getHTML()));
  };

  return (
    <TooltipProvider>
      <Card className="flex flex-col gap-4 p-4 sm:p-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0 flex-1">
            <ContentEditorTitle
              disabled={!canEditContent}
              title={title}
              onTitleChange={onTitleChange}
            />
          </div>
          {viewSwitcher ? <div className="shrink-0">{viewSwitcher}</div> : null}
        </div>

        <ContentEditorDescription
          blockLabel={blockLabel}
          characterCount={content.text.length}
          imageCount={imageCount}
        />

        {collaboration?.error ? (
          <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
            {collaboration.error}
          </div>
        ) : null}

        {collaboration?.role === "viewer" ? (
          <CollaborationReadonlyNotice />
        ) : null}

        <AIEditAssistant
          title={t("ai.editTitle")}
          source={aiSource}
          disabled={!canEditContent || !editor}
          onApply={applyAIProposal}
          onGenerate={(message, onChunk, signal) =>
            streamAIContentEdit(
              {
                content: aiSource,
                message,
                title,
              },
              {
                onChunk,
                signal,
              },
            )
          }
        />

        <input
          ref={fileInputRef}
          type="file"
          accept="image/*"
          multiple
          className="hidden"
          onChange={handleImageSelect}
        />

        <ContentEditorBody
          editor={editor}
          toolbar={
            <ContentEditorToolbar
              editor={editor}
              disabled={!canEditContent}
              onInsertImage={() => fileInputRef.current?.click()}
              onSetLink={setLink}
              trailing={
                collaboration ? (
                  <ContentCollaborationStatus collaboration={collaboration} />
                ) : null
              }
            />
          }
        />
      </Card>
    </TooltipProvider>
  );
}

function CollaborationReadonlyNotice() {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");

  return (
    <div className="rounded-lg border bg-muted/40 p-3 text-sm text-muted-foreground">
      {t("collab.editor.readonly")}
    </div>
  );
}

function ContentCollaborationStatus({
  collaboration,
}: {
  collaboration: ContentEditorCollaboration;
}) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");

  return (
    <>
      <Badge
        variant="outline"
        className={statusClassName(collaboration.status)}
      >
        {t(`collab.status.${collaboration.status}`)}
      </Badge>
      {collaboration.role ? (
        <Badge variant="secondary">
          {t(`collab.role.${collaboration.role}`)}
        </Badge>
      ) : null}
      {collaboration.unsyncedChanges > 0 ? (
        <Badge variant="outline">
          {t("collab.status.unsynced", {
            count: collaboration.unsyncedChanges,
          })}
        </Badge>
      ) : null}
      <Badge variant="outline" className="gap-1">
        <UsersRound className="size-3" />
        {t("collab.status.online", {
          count: collaboration.onlineUsers.length,
        })}
      </Badge>
    </>
  );
}

function statusClassName(status: CollabConnectionStatus) {
  return cn(
    status === "synced" &&
      "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
    status === "connected" &&
      "border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-300",
    (status === "offline" || status === "error" || status === "unauthorized") &&
      "border-destructive/30 bg-destructive/10 text-destructive",
  );
}

function ContentEditorDescription({
  blockLabel,
  characterCount,
  imageCount,
}: {
  blockLabel: string;
  characterCount: number;
  imageCount: number;
}) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");

  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <p className="text-sm text-muted-foreground">{t("editor.desc")}</p>
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span>{blockLabel}</span>
        <span>{t("editor.wordCount", { count: characterCount })}</span>
        <span>{t("editor.imageCount", { count: imageCount })}</span>
      </div>
    </div>
  );
}

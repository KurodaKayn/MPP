"use client";

import { useRef } from "react";

import { AIEditAssistant } from "@/components/dashboard/content/ai/ai-edit-assistant";
import { getCurrentBlockLabel } from "@/components/dashboard/content/editor/content-editor-block-menu";
import {
  ContentEditorBody,
  ContentEditorTitle,
} from "@/components/dashboard/content/editor/content-editor-document";
import { ContentEditorToolbar } from "@/components/dashboard/content/editor/content-editor-toolbar";
import { contentValueFromHtml } from "@/components/dashboard/content/editor/content-editor-utils";
import { useContentTipTapEditor } from "@/components/dashboard/content/editor/use-content-tiptap-editor";
import { Card } from "@/components/ui/card";
import { TooltipProvider } from "@/components/ui/tooltip";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import { streamAIContentEdit } from "@/lib/dashboard/api";
import type { ContentValue } from "@/lib/content/types";

type ContentEditorProps = {
  canEdit?: boolean;
  title: string;
  content: ContentValue;
  onTitleChange: (title: string) => void;
  onContentChange: (content: ContentValue) => void;
  viewSwitcher?: React.ReactNode;
};

export function ContentEditor({
  canEdit = true,
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
      content,
      editable: canEdit,
      onContentChange,
    });
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
              disabled={!canEdit}
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

        <AIEditAssistant
          title={t("ai.editTitle")}
          source={aiSource}
          disabled={!canEdit || !editor}
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
              disabled={!canEdit}
              onInsertImage={() => fileInputRef.current?.click()}
              onSetLink={setLink}
            />
          }
        />
      </Card>
    </TooltipProvider>
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

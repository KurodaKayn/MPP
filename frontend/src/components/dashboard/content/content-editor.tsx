"use client";

import { useRef } from "react";

import { getCurrentBlockLabel } from "@/components/dashboard/content/editor/content-editor-block-menu";
import {
  ContentEditorBody,
  ContentEditorTitle,
} from "@/components/dashboard/content/editor/content-editor-document";
import { ContentEditorToolbar } from "@/components/dashboard/content/editor/content-editor-toolbar";
import { useContentTipTapEditor } from "@/components/dashboard/content/editor/use-content-tiptap-editor";
import { Card } from "@/components/ui/card";
import { TooltipProvider } from "@/components/ui/tooltip";
import type { ContentValue } from "@/lib/content/types";

type ContentEditorProps = {
  title: string;
  content: ContentValue;
  onTitleChange: (title: string) => void;
  onContentChange: (content: ContentValue) => void;
  viewSwitcher?: React.ReactNode;
};

export function ContentEditor({
  title,
  content,
  onTitleChange,
  onContentChange,
  viewSwitcher,
}: ContentEditorProps) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const { editor, handleImageSelect, imageCount, setLink } =
    useContentTipTapEditor({
      content,
      onContentChange,
    });
  const blockLabel = getCurrentBlockLabel(editor);

  return (
    <TooltipProvider>
      <Card className="flex flex-col gap-4 p-4 sm:p-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0 flex-1">
            <ContentEditorTitle title={title} onTitleChange={onTitleChange} />
          </div>
          {viewSwitcher ? <div className="shrink-0">{viewSwitcher}</div> : null}
        </div>

        <ContentEditorDescription
          blockLabel={blockLabel}
          characterCount={content.text.length}
          imageCount={imageCount}
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
  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <p className="text-sm text-muted-foreground">
        像文档一样编写正文，发布前可切换到预览查看效果
      </p>
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span>{blockLabel}</span>
        <span>{characterCount} 字</span>
        <span>{imageCount} 图</span>
      </div>
    </div>
  );
}

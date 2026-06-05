"use client";

import type { Editor } from "@tiptap/react";
import {
  AlignCenter,
  AlignLeft,
  AlignRight,
  Bold,
  Eraser,
  Italic,
  Link2,
  Link2Off,
  Redo2,
  Strikethrough,
  Underline,
  Undo2,
  UsersRound,
} from "lucide-react";
import { ContentEditorBlockMenu } from "@/components/dashboard/content/editor/content-editor-block-menu";
import {
  ToolbarButton,
  ToolbarSeparator,
} from "@/components/dashboard/content/editor/content-editor-toolbar-button";
import { Badge } from "@/components/ui/badge";
import type { CollabDocumentRole } from "@/lib/dashboard/api";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import type {
  CollabConnectionStatus,
  CollabUserProfile,
} from "./collab-provider";
import { collabStatusClassName } from "./collab-status";

type CollabEditorToolbarProps = {
  canEdit: boolean;
  editor: Editor | null;
  onSetLink: () => void;
  onlineUsers: CollabUserProfile[];
  role: CollabDocumentRole | null;
  status: CollabConnectionStatus;
  unsyncedChanges: number;
};

export function CollabEditorToolbar({
  canEdit,
  editor,
  onSetLink,
  onlineUsers,
  role,
  status,
  unsyncedChanges,
}: CollabEditorToolbarProps) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const disabled = !editor || !canEdit;

  return (
    <div className="flex flex-wrap items-center gap-1 rounded-t-[calc(0.75rem-1px)] border-b bg-muted/30 px-3 py-2">
      <ToolbarButton
        label={t("collab.toolbar.undo")}
        disabled={disabled || !editor?.can().undo()}
        onClick={() => editor?.chain().focus().undo().run()}
      >
        <Undo2 className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={t("collab.toolbar.redo")}
        disabled={disabled || !editor?.can().redo()}
        onClick={() => editor?.chain().focus().redo().run()}
      >
        <Redo2 className="size-4" />
      </ToolbarButton>
      <ToolbarSeparator />

      <ContentEditorBlockMenu editor={canEdit ? editor : null} />
      <ToolbarSeparator />

      <ToolbarButton
        label={t("collab.toolbar.bold")}
        active={editor?.isActive("bold")}
        disabled={disabled}
        onClick={() => editor?.chain().focus().toggleBold().run()}
      >
        <Bold className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={t("collab.toolbar.italic")}
        active={editor?.isActive("italic")}
        disabled={disabled}
        onClick={() => editor?.chain().focus().toggleItalic().run()}
      >
        <Italic className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={t("collab.toolbar.underline")}
        active={editor?.isActive("underline")}
        disabled={disabled}
        onClick={() => editor?.chain().focus().toggleUnderline().run()}
      >
        <Underline className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={t("collab.toolbar.strike")}
        active={editor?.isActive("strike")}
        disabled={disabled}
        onClick={() => editor?.chain().focus().toggleStrike().run()}
      >
        <Strikethrough className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={t("collab.toolbar.link")}
        active={editor?.isActive("link")}
        disabled={disabled}
        onClick={onSetLink}
      >
        <Link2 className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={t("collab.toolbar.unlink")}
        disabled={disabled || !editor?.isActive("link")}
        onClick={() =>
          editor?.chain().focus().extendMarkRange("link").unsetLink().run()
        }
      >
        <Link2Off className="size-4" />
      </ToolbarButton>
      <ToolbarSeparator />

      <ToolbarButton
        label={t("collab.toolbar.alignLeft")}
        active={editor?.isActive({ textAlign: "left" })}
        disabled={disabled}
        onClick={() => editor?.chain().focus().setTextAlign("left").run()}
      >
        <AlignLeft className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={t("collab.toolbar.alignCenter")}
        active={editor?.isActive({ textAlign: "center" })}
        disabled={disabled}
        onClick={() => editor?.chain().focus().setTextAlign("center").run()}
      >
        <AlignCenter className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={t("collab.toolbar.alignRight")}
        active={editor?.isActive({ textAlign: "right" })}
        disabled={disabled}
        onClick={() => editor?.chain().focus().setTextAlign("right").run()}
      >
        <AlignRight className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={t("collab.toolbar.clearFormat")}
        disabled={disabled}
        onClick={() =>
          editor?.chain().focus().unsetAllMarks().clearNodes().run()
        }
      >
        <Eraser className="size-4" />
      </ToolbarButton>

      <div className="ml-auto flex flex-wrap items-center gap-2 pl-2">
        <Badge variant="outline" className={collabStatusClassName(status)}>
          {t(`collab.status.${status}`)}
        </Badge>
        {role ? (
          <Badge variant="secondary">{t(`collab.role.${role}`)}</Badge>
        ) : null}
        {unsyncedChanges > 0 ? (
          <Badge variant="outline">
            {t("collab.status.unsynced", { count: unsyncedChanges })}
          </Badge>
        ) : null}
        <Badge variant="outline" className="gap-1">
          <UsersRound className="size-3" />
          {t("collab.status.online", { count: onlineUsers.length })}
        </Badge>
      </div>
    </div>
  );
}

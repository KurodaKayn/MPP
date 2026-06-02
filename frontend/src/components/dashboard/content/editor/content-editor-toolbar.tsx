import type { Editor } from "@tiptap/react";
import {
  AlignCenter,
  AlignLeft,
  AlignRight,
  Bold,
  Eraser,
  ImagePlus,
  Italic,
  Link2,
  Link2Off,
  Redo2,
  Strikethrough,
  Underline,
  Undo2,
} from "lucide-react";

import { ContentEditorBlockMenu } from "@/components/dashboard/content/editor/content-editor-block-menu";
import {
  ToolbarButton,
  ToolbarSeparator,
} from "@/components/dashboard/content/editor/content-editor-toolbar-button";

type ContentEditorToolbarProps = {
  editor: Editor | null;
  onInsertImage: () => void;
  onSetLink: () => void;
};

import { useAppLocale, useTranslation } from "@/lib/i18n/client";
export function ContentEditorToolbar({
  editor,
  onInsertImage,
  onSetLink,
}: ContentEditorToolbarProps) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");

  return (
    <div className="flex flex-wrap items-center gap-1 rounded-t-[calc(0.75rem-1px)] border-b bg-muted/30 px-3 py-2">
      <HistoryControls editor={editor} />
      <ToolbarSeparator />

      <ContentEditorBlockMenu editor={editor} />
      <ToolbarSeparator />

      <InlineFormatControls editor={editor} onSetLink={onSetLink} />
      <ToolbarSeparator />

      <AlignmentControls editor={editor} />
      <ToolbarSeparator />

      <ToolbarButton label={t("toolbar.insertImage")} onClick={onInsertImage}>
        <ImagePlus className="size-4" />
      </ToolbarButton>
    </div>
  );
}

function HistoryControls({ editor }: { editor: Editor | null }) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");

  return (
    <>
      <ToolbarButton
        label={t("toolbar.undo")}
        disabled={!editor?.can().undo()}
        onClick={() => editor?.chain().focus().undo().run()}
      >
        <Undo2 className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={t("toolbar.redo")}
        disabled={!editor?.can().redo()}
        onClick={() => editor?.chain().focus().redo().run()}
      >
        <Redo2 className="size-4" />
      </ToolbarButton>
    </>
  );
}

function InlineFormatControls({
  editor,
  onSetLink,
}: {
  editor: Editor | null;
  onSetLink: () => void;
}) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");

  return (
    <>
      <ToolbarButton
        label={t("toolbar.bold")}
        active={editor?.isActive("bold")}
        onClick={() => editor?.chain().focus().toggleBold().run()}
      >
        <Bold className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={t("toolbar.italic")}
        active={editor?.isActive("italic")}
        onClick={() => editor?.chain().focus().toggleItalic().run()}
      >
        <Italic className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={t("toolbar.underline")}
        active={editor?.isActive("underline")}
        onClick={() => editor?.chain().focus().toggleUnderline().run()}
      >
        <Underline className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={t("toolbar.strike")}
        active={editor?.isActive("strike")}
        onClick={() => editor?.chain().focus().toggleStrike().run()}
      >
        <Strikethrough className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={
          editor?.isActive("link")
            ? t("toolbar.editLink")
            : t("toolbar.insertLink")
        }
        active={editor?.isActive("link")}
        onClick={onSetLink}
      >
        <Link2 className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={t("toolbar.removeLink")}
        disabled={!editor?.isActive("link")}
        onClick={() =>
          editor?.chain().focus().extendMarkRange("link").unsetLink().run()
        }
      >
        <Link2Off className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={t("toolbar.clearFormat")}
        onClick={() =>
          editor?.chain().focus().unsetAllMarks().clearNodes().run()
        }
      >
        <Eraser className="size-4" />
      </ToolbarButton>
    </>
  );
}

function AlignmentControls({ editor }: { editor: Editor | null }) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");

  return (
    <>
      <ToolbarButton
        label={t("toolbar.alignLeft")}
        active={editor?.isActive({ textAlign: "left" })}
        onClick={() => editor?.chain().focus().setTextAlign("left").run()}
      >
        <AlignLeft className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={t("toolbar.alignCenter")}
        active={editor?.isActive({ textAlign: "center" })}
        onClick={() => editor?.chain().focus().setTextAlign("center").run()}
      >
        <AlignCenter className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label={t("toolbar.alignRight")}
        active={editor?.isActive({ textAlign: "right" })}
        onClick={() => editor?.chain().focus().setTextAlign("right").run()}
      >
        <AlignRight className="size-4" />
      </ToolbarButton>
    </>
  );
}

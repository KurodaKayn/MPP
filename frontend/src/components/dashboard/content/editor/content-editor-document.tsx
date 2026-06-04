import type { ReactNode } from "react";
import { EditorContent, type Editor } from "@tiptap/react";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import styles from "./content-editor.module.css";

type ContentEditorDocumentProps = {
  editor: Editor | null;
  toolbar: ReactNode;
};

type ContentEditorTitleProps = {
  disabled?: boolean;
  title: string;
  onTitleChange: (title: string) => void;
};

export function ContentEditorTitle({
  disabled = false,
  title,
  onTitleChange,
}: ContentEditorTitleProps) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");

  return (
    <div className="flex items-baseline gap-2">
      <Label
        htmlFor="title"
        className="shrink-0 text-2xl font-semibold tracking-normal sm:text-3xl"
      >
        {t("editor.titleLabel")}
      </Label>
      <Input
        id="title"
        placeholder={t("editor.titlePlaceholder")}
        value={title}
        disabled={disabled}
        className="h-auto border-0 px-0 py-0 text-2xl font-semibold shadow-none focus-visible:ring-0 sm:text-3xl"
        onChange={(event) => onTitleChange(event.target.value)}
      />
    </div>
  );
}

export function ContentEditorBody({
  editor,
  toolbar,
}: ContentEditorDocumentProps) {
  return (
    <div className={styles.body}>
      {toolbar}
      <div className={styles.editorArea}>
        <EditorContent editor={editor} />
      </div>
    </div>
  );
}

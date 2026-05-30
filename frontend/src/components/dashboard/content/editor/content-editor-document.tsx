import type { ReactNode } from "react";
import { EditorContent, type Editor } from "@tiptap/react";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import styles from "./content-editor.module.css";

type ContentEditorDocumentProps = {
  editor: Editor | null;
  toolbar: ReactNode;
};

type ContentEditorTitleProps = {
  title: string;
  onTitleChange: (title: string) => void;
};

export function ContentEditorTitle({
  title,
  onTitleChange,
}: ContentEditorTitleProps) {
  return (
    <div className="flex items-baseline gap-2">
      <Label
        htmlFor="title"
        className="shrink-0 text-2xl font-semibold tracking-normal sm:text-3xl"
      >
        标题：
      </Label>
      <Input
        id="title"
        placeholder="输入文章标题..."
        value={title}
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

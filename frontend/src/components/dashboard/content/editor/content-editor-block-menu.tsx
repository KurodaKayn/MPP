import type { Editor } from "@tiptap/react";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import {
  ChevronDown,
  Heading1,
  Heading2,
  Heading3,
  List,
  ListOrdered,
  Pilcrow,
  Quote,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export function ContentEditorBlockMenu({ editor }: { editor: Editor | null }) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");
  const blockLabel = getCurrentBlockLabel(editor, t);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="min-w-28 justify-between bg-background"
            onMouseDown={(event) => event.preventDefault()}
          >
            <span className="inline-flex items-center gap-1.5">
              <Pilcrow className="size-3.5" />
              {blockLabel}
            </span>
            <ChevronDown className="size-3.5 text-muted-foreground" />
          </Button>
        }
      />
      <DropdownMenuContent className="w-48">
        <DropdownMenuItem
          onClick={() => editor?.chain().focus().setParagraph().run()}
        >
          <Pilcrow className="size-4" />
          {t("editor.paragraph")}
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() =>
            editor?.chain().focus().toggleHeading({ level: 1 }).run()
          }
        >
          <Heading1 className="size-4" />
          {t("editor.h1")}
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() =>
            editor?.chain().focus().toggleHeading({ level: 2 }).run()
          }
        >
          <Heading2 className="size-4" />
          {t("editor.h2")}
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() =>
            editor?.chain().focus().toggleHeading({ level: 3 }).run()
          }
        >
          <Heading3 className="size-4" />
          {t("editor.h3")}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={() => editor?.chain().focus().toggleBulletList().run()}
        >
          <List className="size-4" />
          {t("editor.bulletList")}
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => editor?.chain().focus().toggleOrderedList().run()}
        >
          <ListOrdered className="size-4" />
          {t("editor.orderedList")}
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => editor?.chain().focus().toggleBlockquote().run()}
        >
          <Quote className="size-4" />
          {t("editor.quote")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function getCurrentBlockLabel(
  editor: Editor | null,
  t: (key: string) => string,
) {
  if (!editor) {
    return t("editor.paragraph");
  }

  if (editor.isActive("heading", { level: 1 })) {
    return t("editor.h1");
  }

  if (editor.isActive("heading", { level: 2 })) {
    return t("editor.h2");
  }

  if (editor.isActive("heading", { level: 3 })) {
    return t("editor.h3");
  }

  if (editor.isActive("bulletList")) {
    return t("editor.bulletList");
  }

  if (editor.isActive("orderedList")) {
    return t("editor.orderedList");
  }

  if (editor.isActive("blockquote")) {
    return t("editor.quote");
  }

  return t("editor.paragraph");
}

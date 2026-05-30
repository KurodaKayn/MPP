import type { Editor } from "@tiptap/react";
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
  const blockLabel = getCurrentBlockLabel(editor);

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
          正文
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() =>
            editor?.chain().focus().toggleHeading({ level: 1 }).run()
          }
        >
          <Heading1 className="size-4" />
          标题 1
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() =>
            editor?.chain().focus().toggleHeading({ level: 2 }).run()
          }
        >
          <Heading2 className="size-4" />
          标题 2
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() =>
            editor?.chain().focus().toggleHeading({ level: 3 }).run()
          }
        >
          <Heading3 className="size-4" />
          标题 3
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={() => editor?.chain().focus().toggleBulletList().run()}
        >
          <List className="size-4" />
          项目符号
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => editor?.chain().focus().toggleOrderedList().run()}
        >
          <ListOrdered className="size-4" />
          编号列表
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => editor?.chain().focus().toggleBlockquote().run()}
        >
          <Quote className="size-4" />
          引用
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function getCurrentBlockLabel(editor: Editor | null) {
  if (!editor) {
    return "正文";
  }

  if (editor.isActive("heading", { level: 1 })) {
    return "标题 1";
  }

  if (editor.isActive("heading", { level: 2 })) {
    return "标题 2";
  }

  if (editor.isActive("heading", { level: 3 })) {
    return "标题 3";
  }

  if (editor.isActive("bulletList")) {
    return "项目符号";
  }

  if (editor.isActive("orderedList")) {
    return "编号列表";
  }

  if (editor.isActive("blockquote")) {
    return "引用";
  }

  return "正文";
}

import { useRef } from "react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ImagePlus } from "lucide-react";
import { toast } from "sonner";
import type { ContentValue } from "./types";

type ContentEditorProps = {
  title: string;
  content: ContentValue;
  onTitleChange: (title: string) => void;
  onContentChange: (content: ContentValue) => void;
};

export function ContentEditor({
  title,
  content,
  onTitleChange,
  onContentChange,
}: ContentEditorProps) {
  const editorRef = useRef<HTMLDivElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const savedRangeRef = useRef<Range | null>(null);
  const hasBodyContent = Boolean(content.text.trim() || content.firstImageSrc);

  const syncContentState = () => {
    const editor = editorRef.current;

    if (!editor) {
      return;
    }

    onContentChange({
      html: editor.innerHTML,
      text: editor.innerText.trim(),
      firstImageSrc: editor.querySelector("img")?.getAttribute("src") ?? "",
    });
  };

  const saveSelection = () => {
    const editor = editorRef.current;
    const selection = window.getSelection();

    if (!editor || !selection || selection.rangeCount === 0) {
      return;
    }

    const range = selection.getRangeAt(0);

    if (editor.contains(range.commonAncestorContainer)) {
      savedRangeRef.current = range.cloneRange();
    }
  };

  const restoreSelection = () => {
    const editor = editorRef.current;
    const selection = window.getSelection();

    if (!editor || !selection) {
      return;
    }

    editor.focus();

    if (!savedRangeRef.current) {
      const range = document.createRange();
      range.selectNodeContents(editor);
      range.collapse(false);
      savedRangeRef.current = range;
    }

    selection.removeAllRanges();
    selection.addRange(savedRangeRef.current);
  };

  const insertImageAtCursor = (file: File, src: string) => {
    restoreSelection();

    const selection = window.getSelection();

    if (!selection || selection.rangeCount === 0) {
      return;
    }

    const range = selection.getRangeAt(0);
    const figure = document.createElement("figure");
    const image = document.createElement("img");
    const caption = document.createElement("figcaption");

    figure.contentEditable = "false";
    figure.dataset.contentImage = "true";
    figure.className =
      "my-4 overflow-hidden rounded-lg border bg-muted outline-none";

    image.src = src;
    image.alt = file.name;
    image.className = "max-h-80 w-full object-contain";

    caption.textContent = file.name;
    caption.className =
      "border-t bg-background/90 px-3 py-2 text-xs text-muted-foreground";

    figure.append(image, caption);
    range.deleteContents();
    range.insertNode(figure);

    const nextRange = document.createRange();
    nextRange.setStartAfter(figure);
    nextRange.collapse(true);
    selection.removeAllRanges();
    selection.addRange(nextRange);
    savedRangeRef.current = nextRange.cloneRange();
    syncContentState();
  };

  const handleImageSelect = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";

    if (!file) {
      return;
    }

    if (!file.type.startsWith("image/")) {
      toast.error("请选择图片文件");
      return;
    }

    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result === "string") {
        insertImageAtCursor(file, reader.result);
      }
    };
    reader.readAsDataURL(file);
  };

  return (
    <Card className="flex flex-col">
      <CardHeader>
        <CardTitle>编辑器</CardTitle>
        <CardDescription>编写您的通用内容</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4 flex-1">
        <div className="space-y-2">
          <Label htmlFor="title">标题</Label>
          <Input
            id="title"
            placeholder="输入文章标题..."
            value={title}
            onChange={(event) => onTitleChange(event.target.value)}
          />
        </div>
        <div className="space-y-2 flex-1 flex flex-col">
          <div className="flex items-center justify-between gap-3">
            <Label htmlFor="content-editor">正文</Label>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                saveSelection();
                fileInputRef.current?.click();
              }}
            >
              <ImagePlus className="mr-2 h-4 w-4" /> 插入图片
            </Button>
            <input
              ref={fileInputRef}
              type="file"
              accept="image/*"
              className="hidden"
              onChange={handleImageSelect}
            />
          </div>
          <div className="relative">
            {!hasBodyContent ? (
              <span className="pointer-events-none absolute left-4 top-3 text-sm text-muted-foreground">
                在这里开始创作...
              </span>
            ) : null}
            <div
              ref={editorRef}
              id="content-editor"
              role="textbox"
              aria-multiline="true"
              contentEditable
              suppressContentEditableWarning
              className="min-h-[400px] rounded-lg border border-input bg-transparent px-4 py-3 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 [&_figcaption]:border-t [&_figcaption]:bg-background/90 [&_figcaption]:px-3 [&_figcaption]:py-2 [&_figcaption]:text-xs [&_figcaption]:text-muted-foreground [&_figure]:my-4 [&_figure]:overflow-hidden [&_figure]:rounded-lg [&_figure]:border [&_figure]:bg-muted [&_img]:max-h-80 [&_img]:w-full [&_img]:object-contain"
              onBlur={saveSelection}
              onInput={syncContentState}
              onKeyUp={saveSelection}
              onMouseUp={saveSelection}
            />
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

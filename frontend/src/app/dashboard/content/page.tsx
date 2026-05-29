"use client";

import { useState } from "react";
import { toast } from "sonner";
import { ContentEditor } from "@/components/dashboard/content/content-editor";
import { emptyContentValue, type ContentValue } from "@/lib/content/types";
import { ContentPageHeader } from "./_components/content-page-header";
import { PlatformPreview } from "./_components/platform-preview";

export default function ContentPage() {
  const [content, setContent] = useState<ContentValue>(emptyContentValue);
  const [title, setTitle] = useState("");
  const hasBodyContent = Boolean(content.text.trim() || content.firstImageSrc);

  const handlePublish = () => {
    if (!title || !hasBodyContent) {
      toast.error("内容不完整", {
        description: "请填写标题和正文后再发布。",
      });
      return;
    }
    toast.success("发布中...", {
      description: "内容正在同步到后端服务。",
    });
  };

  return (
    <div className="flex flex-col gap-6">
      <ContentPageHeader onPublish={handlePublish} />

      <div className="grid gap-6 lg:grid-cols-2">
        <ContentEditor
          title={title}
          content={content}
          onTitleChange={setTitle}
          onContentChange={setContent}
        />
        <PlatformPreview title={title} content={content} />
      </div>
    </div>
  );
}

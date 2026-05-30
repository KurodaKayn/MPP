"use client";

import { useRef, useState } from "react";
import { toast } from "sonner";
import { ContentEditor } from "@/components/dashboard/content/content-editor";
import { emptyContentValue, type ContentValue } from "@/lib/content/types";
import { PLATFORM_TABS, type PlatformTab } from "@/lib/content/platforms";
import { ContentPageHeader } from "./_components/content-page-header";
import { ContentPublishBar } from "./_components/content-publish-bar";
import { PlatformPreview } from "./_components/platform-preview";

type PublishPlatform = PlatformTab["value"];

export default function ContentPage() {
  const [content, setContent] = useState<ContentValue>(emptyContentValue);
  const [selectedPlatforms, setSelectedPlatforms] = useState<PublishPlatform[]>(
    [],
  );
  const [title, setTitle] = useState("");
  const publishBarRef = useRef<HTMLDivElement>(null);
  const hasBodyContent = Boolean(content.text.trim() || content.firstImageSrc);
  const canPublish = Boolean(
    title.trim() && hasBodyContent && selectedPlatforms.length > 0,
  );

  const handleOpenPublishPanel = () => {
    publishBarRef.current?.scrollIntoView({
      behavior: "smooth",
      block: "end",
    });
  };

  const handlePublish = () => {
    if (selectedPlatforms.length === 0) {
      toast.error("请选择发布平台", {
        description: "在底部发布渠道中勾选至少一个平台。",
      });
      return;
    }

    if (!title.trim() || !hasBodyContent) {
      toast.error("内容不完整", {
        description: "请填写标题和正文后再发布。",
      });
      return;
    }

    const selectedLabels = PLATFORM_TABS.filter((platform) =>
      selectedPlatforms.includes(platform.value),
    ).map((platform) => platform.label);

    toast.success("发布中...", {
      description: `内容将发布到 ${selectedLabels.join("、")}。`,
    });
  };

  return (
    <div className="flex flex-col gap-6 pb-4">
      <ContentPageHeader onOpenPublishPanel={handleOpenPublishPanel} />

      <div className="grid gap-6 lg:grid-cols-2">
        <ContentEditor
          title={title}
          content={content}
          onTitleChange={setTitle}
          onContentChange={setContent}
        />
        <PlatformPreview title={title} content={content} />
      </div>

      <div ref={publishBarRef}>
        <ContentPublishBar
          canPublish={canPublish}
          selectedPlatforms={selectedPlatforms}
          onSelectedPlatformsChange={setSelectedPlatforms}
          onPublish={handlePublish}
        />
      </div>
    </div>
  );
}

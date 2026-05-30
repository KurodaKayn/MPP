"use client";

import { useRef, useState } from "react";
import { toast } from "sonner";
import { ContentEditor } from "@/components/dashboard/content/content-editor";
import { emptyContentValue, type ContentValue } from "@/lib/content/types";
import { PLATFORM_TABS, type PlatformTab } from "@/lib/content/platforms";
import { createDashboardProject, publishProject } from "@/lib/dashboard/api";
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
  const [isPublishing, setIsPublishing] = useState(false);
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

  const getSelectedPlatformLabels = (platforms: PublishPlatform[]) =>
    PLATFORM_TABS.filter((platform) => platforms.includes(platform.value)).map(
      (platform) => platform.label,
    );

  const handlePublish = async () => {
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

    const sourceContent = content.html || content.text;
    const selectedLabels = getSelectedPlatformLabels(selectedPlatforms);

    setIsPublishing(true);
    try {
      const project = await createDashboardProject({
        cover_image_url: content.firstImageSrc || undefined,
        platforms: selectedPlatforms,
        source_content: sourceContent,
        summary: content.text,
        title: title.trim(),
      });

      const results = await Promise.allSettled(
        selectedPlatforms.map(async (platform) => {
          const result = await publishProject(project.id, platform);
          if (result.status === "failed") {
            throw new Error(result.error_message || `${platform} 发布失败`);
          }
          return { platform, result };
        }),
      );
      const failed = results.filter((result) => result.status === "rejected");
      const succeeded = results.filter(
        (result) => result.status === "fulfilled",
      );

      if (failed.length > 0) {
        const firstError = failed[0].reason;
        toast.error(succeeded.length > 0 ? "部分平台发布失败" : "发布失败", {
          description:
            firstError instanceof Error ? firstError.message : "请稍后重试。",
        });
        return;
      }

      toast.success("发布完成", {
        description: `已发布到 ${selectedLabels.join("、")}。`,
      });
    } catch (requestError) {
      toast.error("发布请求失败", {
        description:
          requestError instanceof Error ? requestError.message : "请稍后重试。",
      });
    } finally {
      setIsPublishing(false);
    }
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
          isPublishing={isPublishing}
          selectedPlatforms={selectedPlatforms}
          onSelectedPlatformsChange={setSelectedPlatforms}
          onPublish={() => void handlePublish()}
        />
      </div>
    </div>
  );
}

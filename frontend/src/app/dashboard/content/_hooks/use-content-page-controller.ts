"use client";

import { useRef, useState } from "react";
import { toast } from "sonner";
import { PLATFORM_TABS } from "@/lib/content/platforms";
import { emptyContentValue, type ContentValue } from "@/lib/content/types";
import { createDashboardProject, publishProject } from "@/lib/dashboard/api";
import {
  publishContentToPlatforms,
  type PublishPlatform,
} from "../_lib/publish-content";

export function useContentPageController() {
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

  const getSelectedPlatformLabels = (platforms: PublishPlatform[]) =>
    PLATFORM_TABS.filter((platform) => platforms.includes(platform.value)).map(
      (platform) => platform.label,
    );

  const openPublishPanel = () => {
    publishBarRef.current?.scrollIntoView({
      behavior: "smooth",
      block: "end",
    });
  };

  const publish = async () => {
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

    setIsPublishing(true);
    try {
      const result = await publishContentToPlatforms(
        {
          content,
          platforms: selectedPlatforms,
          title: title.trim(),
        },
        {
          createProject: createDashboardProject,
          publishProject,
        },
      );

      if (result.failed.length > 0) {
        toast.error(
          result.succeeded.length > 0 ? "部分平台发布失败" : "发布失败",
          {
            description: result.failed[0].message,
          },
        );
        return;
      }

      toast.success("发布完成", {
        description: `已发布到 ${getSelectedPlatformLabels(
          result.succeeded,
        ).join("、")}。`,
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

  return {
    canPublish,
    content,
    isPublishing,
    openPublishPanel,
    publish: () => void publish(),
    publishBarRef,
    selectedPlatforms,
    setContent,
    setSelectedPlatforms,
    setTitle,
    title,
  };
}

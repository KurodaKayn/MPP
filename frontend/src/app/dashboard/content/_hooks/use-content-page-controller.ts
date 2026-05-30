"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { PLATFORM_TABS } from "@/lib/content/platforms";
import { emptyContentValue, type ContentValue } from "@/lib/content/types";
import {
  createDashboardProject,
  getDashboardProject,
  publishProject,
  updateDashboardProject,
  type CreateProjectInput,
} from "@/lib/dashboard/api";
import {
  publishContentToPlatforms,
  type PublishPlatform,
} from "../_lib/publish-content";

function isPublishPlatform(platform: string): platform is PublishPlatform {
  return PLATFORM_TABS.some((item) => item.value === platform);
}

function contentValueFromSource(sourceContent: string): ContentValue {
  const container = document.createElement("div");
  container.innerHTML = sourceContent;

  return {
    firstImageSrc: container.querySelector("img")?.getAttribute("src") ?? "",
    html: sourceContent,
    text: container.innerText.trim() || sourceContent.trim(),
  };
}

export function useContentPageController(projectId?: string) {
  const router = useRouter();
  const [content, setContent] = useState<ContentValue>(emptyContentValue);
  const [selectedPlatforms, setSelectedPlatforms] = useState<PublishPlatform[]>(
    [],
  );
  const [title, setTitle] = useState("");
  const [isLoading, setIsLoading] = useState(Boolean(projectId));
  const [isOpeningXPostIntent, setIsOpeningXPostIntent] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [isPublishing, setIsPublishing] = useState(false);
  const publishBarRef = useRef<HTMLDivElement>(null);
  const hasBodyContent = Boolean(content.text.trim() || content.firstImageSrc);
  const hasRequiredContent = Boolean(
    !isLoading && title.trim() && hasBodyContent,
  );
  const canPublish = Boolean(
    hasRequiredContent && selectedPlatforms.length > 0,
  );
  const canSave = Boolean(projectId && canPublish);
  const canOpenXPostIntent = hasRequiredContent;

  useEffect(() => {
    if (!projectId) {
      setIsLoading(false);
      return;
    }

    const targetProjectId = projectId;
    let cancelled = false;

    async function loadProject() {
      setIsLoading(true);
      try {
        const project = await getDashboardProject(targetProjectId);
        if (cancelled) {
          return;
        }

        setTitle(project.title);
        setContent(contentValueFromSource(project.source_content));
        setSelectedPlatforms(
          project.publications.flatMap((publication) =>
            publication.enabled && isPublishPlatform(publication.platform)
              ? [publication.platform]
              : [],
          ),
        );
      } catch (requestError) {
        toast.error("无法加载项目内容", {
          description:
            requestError instanceof Error
              ? requestError.message
              : "请稍后重试。",
        });
      } finally {
        if (!cancelled) {
          setIsLoading(false);
        }
      }
    }

    void loadProject();

    return () => {
      cancelled = true;
    };
  }, [projectId]);

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

  const validateContentFields = () => {
    if (!title.trim() || !hasBodyContent) {
      toast.error("内容不完整", {
        description: "请填写标题和正文后再发布。",
      });
      return false;
    }

    return true;
  };

  const validateContent = () => {
    if (selectedPlatforms.length === 0) {
      toast.error("请选择发布平台", {
        description: "在底部发布渠道中勾选至少一个平台。",
      });
      return false;
    }

    return validateContentFields();
  };

  const buildProjectInput = (
    platforms: PublishPlatform[] = selectedPlatforms,
  ): CreateProjectInput => ({
    cover_image_url: content.firstImageSrc || undefined,
    platforms,
    source_content: content.html || content.text,
    summary: content.text,
    title: title.trim(),
  });

  const saveOrCreateProjectForXPostIntent = async () => {
    const platforms: PublishPlatform[] = selectedPlatforms.includes("x")
      ? selectedPlatforms
      : [...selectedPlatforms, "x"];
    const input = buildProjectInput(platforms);

    if (projectId) {
      await updateDashboardProject(projectId, input);
      return { id: projectId, isNew: false };
    }

    const project = await createDashboardProject(input);
    return { id: project.id, isNew: true };
  };

  const save = async () => {
    if (!projectId || !validateContent()) {
      return;
    }

    setIsSaving(true);
    try {
      await updateDashboardProject(projectId, buildProjectInput());
      toast.success("修改已保存");
    } catch (requestError) {
      toast.error("保存失败", {
        description:
          requestError instanceof Error ? requestError.message : "请稍后重试。",
      });
    } finally {
      setIsSaving(false);
    }
  };

  const publishExistingProject = async () => {
    if (!projectId) {
      return;
    }

    await updateDashboardProject(projectId, buildProjectInput());
    const results = await Promise.allSettled(
      selectedPlatforms.map(async (platform) => {
        const result = await publishProject(projectId, platform);
        if (result.status === "failed") {
          throw new Error(result.error_message || `${platform} 发布失败`);
        }
        return platform;
      }),
    );

    const succeeded: PublishPlatform[] = [];
    const failed: { message: string; platform: PublishPlatform }[] = [];

    results.forEach((result, index) => {
      const platform = selectedPlatforms[index];
      if (result.status === "fulfilled") {
        succeeded.push(result.value);
        return;
      }

      failed.push({
        message:
          result.reason instanceof Error
            ? result.reason.message
            : "请稍后重试。",
        platform,
      });
    });

    return { failed, succeeded };
  };

  const openXPostIntent = async () => {
    if (!validateContentFields()) {
      return;
    }

    const popup = window.open("about:blank", "_blank");
    if (popup) {
      popup.document.title = "Opening X";
      popup.opener = null;
    }

    setIsOpeningXPostIntent(true);
    try {
      const targetProject = await saveOrCreateProjectForXPostIntent();
      const result = await publishProject(targetProject.id, "x", {
        mode: "manual",
      });
      if (!result.publish_url) {
        throw new Error("后端没有返回 X 发帖链接");
      }

      if (popup) {
        popup.location.href = result.publish_url;
      } else {
        const opened = window.open(
          result.publish_url,
          "_blank",
          "noopener,noreferrer",
        );
        if (!opened) {
          await navigator.clipboard?.writeText(result.publish_url);
          toast.error("浏览器拦截了新窗口", {
            description: "X 发帖链接已复制，请粘贴到浏览器打开。",
          });
          return;
        }
      }

      toast.success("X 发帖窗口已打开", {
        description: "确认内容后在 X 页面点击 Post。",
      });
      if (targetProject.isNew) {
        router.replace(`/dashboard/content/${targetProject.id}`);
      }
    } catch (requestError) {
      popup?.close();
      toast.error("无法打开 X 发帖窗口", {
        description:
          requestError instanceof Error ? requestError.message : "请稍后重试。",
      });
    } finally {
      setIsOpeningXPostIntent(false);
    }
  };

  const publish = async () => {
    if (!validateContent()) {
      return;
    }

    setIsPublishing(true);
    try {
      const result = projectId
        ? await publishExistingProject()
        : await publishContentToPlatforms(
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

      if (!result) {
        return;
      }

      if (result.failed.length > 0) {
        toast.error(
          result.succeeded.length > 0 ? "部分平台发布失败" : "发布失败",
          {
            description: result.failed[0].message,
          },
        );
        return;
      }

      toast.success(projectId ? "修改并发布完成" : "发布完成", {
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
    canSave,
    canOpenXPostIntent,
    canPublish,
    content,
    isEditing: Boolean(projectId),
    isLoading,
    isOpeningXPostIntent,
    isPublishing,
    isSaving,
    openPublishPanel,
    openXPostIntent: () => void openXPostIntent(),
    publish: () => void publish(),
    publishBarRef,
    save: () => void save(),
    selectedPlatforms,
    setContent,
    setSelectedPlatforms,
    setTitle,
    title,
  };
}

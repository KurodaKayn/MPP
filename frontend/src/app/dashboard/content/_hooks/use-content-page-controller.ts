"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef } from "react";
import { toast } from "sonner";
import { PLATFORM_TABS } from "@/lib/content/platforms";
import { emptyContentValue, type ContentValue } from "@/lib/content/types";
import {
  createDashboardProject,
  getDashboardProject,
  getProjectPublications,
  publishProject,
  syncProjectPrepublish,
  updateDashboardProject,
  type AdaptedContent,
  type CreateProjectInput,
  type ProjectPublications,
} from "@/lib/dashboard/api";
import { type PublishPlatform } from "../_lib/publish-content";
import {
  type PrepublishDraft,
  useContentPageStore,
} from "../_stores/content-page-store";

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

function isPrepublishFormat(
  format: AdaptedContent["format"],
): format is PrepublishDraft["format"] {
  return format === "html" || format === "markdown" || format === "text";
}

function draftsFromPublications(publications: ProjectPublications) {
  return publications.items.reduce<
    Partial<Record<PublishPlatform, PrepublishDraft>>
  >((drafts, publication) => {
    if (!publication.enabled || !isPublishPlatform(publication.platform)) {
      return drafts;
    }

    const adaptedContent = publication.adapted_content;
    if (!isPrepublishFormat(adaptedContent.format)) {
      return drafts;
    }

    const raw =
      adaptedContent.html ??
      adaptedContent.markdown ??
      adaptedContent.text ??
      adaptedContent.summary ??
      "";
    if (!raw) {
      return drafts;
    }

    drafts[publication.platform] = {
      format: adaptedContent.format,
      raw,
      syncedAt:
        adaptedContent.source_revision ??
        publication.updated_at ??
        new Date().toISOString(),
    };
    return drafts;
  }, {});
}

export function useContentPageController(projectId?: string) {
  const router = useRouter();
  const {
    content,
    isLoading,
    isOpeningXPostIntent,
    isPublishing,
    isSaving,
    isSyncingPrepublish,
    loadedProjectId,
    prepublishDrafts,
    resetForCreate,
    selectedPlatforms,
    setContent,
    setIsLoading,
    setIsOpeningXPostIntent,
    setIsPublishing,
    setIsSaving,
    setIsSyncingPrepublish,
    setLoadedProjectId,
    setPrepublishDrafts,
    setSelectedPlatforms,
    setTitle,
    title,
  } = useContentPageStore();
  const publishBarRef = useRef<HTMLDivElement>(null);
  const isRouteContentLoaded = projectId
    ? loadedProjectId === projectId
    : loadedProjectId === null;
  const isPageLoading = isLoading || !isRouteContentLoaded;
  const hasBodyContent = Boolean(content.text.trim() || content.firstImageSrc);
  const hasRequiredContent = Boolean(
    !isPageLoading && title.trim() && hasBodyContent,
  );
  const hasSyncedSelectedPlatforms = selectedPlatforms.every(
    (platform) => prepublishDrafts[platform],
  );
  const canPublish = Boolean(
    projectId &&
    hasRequiredContent &&
    selectedPlatforms.length > 0 &&
    hasSyncedSelectedPlatforms,
  );
  const canSave = Boolean(
    projectId && hasRequiredContent && selectedPlatforms.length > 0,
  );
  const canOpenXPostIntent = hasRequiredContent;

  useEffect(() => {
    if (!projectId) {
      resetForCreate();
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
        const publications = await getProjectPublications(targetProjectId, {
          includeContent: true,
        });
        if (cancelled) {
          return;
        }

        setPrepublishDrafts(draftsFromPublications(publications));
        setLoadedProjectId(targetProjectId);
      } catch (requestError) {
        if (cancelled) {
          return;
        }

        setTitle("");
        setContent(emptyContentValue);
        setSelectedPlatforms([]);
        setPrepublishDrafts({});
        setLoadedProjectId(targetProjectId);
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
      setPrepublishDrafts({});
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

  const syncPrepublish = async () => {
    if (!validateContent()) {
      return;
    }

    setIsSyncingPrepublish(true);
    try {
      const targetProject = projectId
        ? await updateDashboardProject(projectId, buildProjectInput())
        : await createDashboardProject(buildProjectInput());
      const publications = await syncProjectPrepublish(targetProject.id, {
        platforms: selectedPlatforms,
      });

      setPrepublishDrafts(draftsFromPublications(publications));
      toast.success("已同步到预发布", {
        description: "平台草稿已由后端适配并保存。",
      });
      if (!projectId) {
        router.replace(`/dashboard/content/${targetProject.id}`);
      }
    } catch (requestError) {
      toast.error("同步到预发布失败", {
        description:
          requestError instanceof Error ? requestError.message : "请稍后重试。",
      });
    } finally {
      setIsSyncingPrepublish(false);
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
    if (!canPublish) {
      toast.error("请先同步到预发布", {
        description: "发布前需要为所有选中平台生成平台草稿。",
      });
      return;
    }

    setIsPublishing(true);
    try {
      const result = await publishExistingProject();

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
    isLoading: isPageLoading,
    isOpeningXPostIntent,
    isPublishing,
    isSaving,
    isSyncingPrepublish,
    openPublishPanel,
    openXPostIntent: () => void openXPostIntent(),
    prepublishDrafts,
    publish: () => void publish(),
    publishBarRef,
    save: () => void save(),
    selectedPlatforms,
    setContent: (nextContent: ContentValue) => {
      setContent(nextContent);
      setPrepublishDrafts({});
    },
    setSelectedPlatforms,
    setTitle: (nextTitle: string) => {
      setTitle(nextTitle);
      setPrepublishDrafts({});
    },
    syncPrepublish,
    title,
  };
}

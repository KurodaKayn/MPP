"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { PLATFORM_TABS } from "@/lib/content/platforms";
import { emptyContentValue, type ContentValue } from "@/lib/content/types";
import {
  getDashboardProject,
  getProjectPublications,
} from "@/lib/dashboard/api";
import type { ProjectRole } from "@/lib/dashboard/api";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import { type PublishPlatform } from "../_lib/publish-content";
import { useContentPageStore } from "../_stores/content-page-store";
import {
  draftsFromPublications,
  useContentPublishWorkflow,
} from "../_workflows/use-content-publish-workflow";

function isPublishPlatform(platform: string): platform is PublishPlatform {
  return PLATFORM_TABS.some((item) => item.value === platform);
}

function contentValueFromSource(sourceContent: string): ContentValue {
  const container = document.createElement("div");
  container.innerHTML = sourceContent;

  return {
    firstImageSrc: container.querySelector("img")?.getAttribute("src") ?? "",
    html: sourceContent,
    text:
      container.innerText?.trim() ||
      container.textContent?.trim() ||
      sourceContent.trim(),
  };
}

export function useContentPageController(projectId?: string) {
  const router = useRouter();
  const {
    content,
    isLoading,
    isOpeningXPostIntent,
    isSaving,
    isSyncingPrepublish,
    loadedProjectId,
    prepublishDrafts,
    resetForCreate,
    selectedPlatforms,
    setContent,
    setIsLoading,
    setIsOpeningXPostIntent,
    setIsSaving,
    setIsSyncingPrepublish,
    setLoadedProjectId,
    setPrepublishDrafts,
    setSelectedPlatforms,
    setTitle,
    title,
  } = useContentPageStore();
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");
  const [projectRole, setProjectRole] = useState<ProjectRole | null>(null);
  const publishBarRef = useRef<HTMLDivElement>(null);
  const isRouteContentLoaded = projectId
    ? loadedProjectId === projectId
    : loadedProjectId === null;
  const isPageLoading = isLoading || !isRouteContentLoaded;
  const hasBodyContent = Boolean(content.text.trim() || content.firstImageSrc);
  const hasRequiredContent = Boolean(
    !isPageLoading && title.trim() && hasBodyContent,
  );
  const automaticPublishPlatforms = selectedPlatforms.filter(
    (platform) => platform !== "douyin",
  );
  const hasSyncedSelectedPlatforms = automaticPublishPlatforms.every(
    (platform) => prepublishDrafts[platform],
  );
  const canEditProject = !projectId || projectRole !== "viewer";
  const canPublishProject = !projectId || projectRole === "owner";
  const canPublish = Boolean(
    projectId &&
    canPublishProject &&
    hasRequiredContent &&
    automaticPublishPlatforms.length > 0 &&
    hasSyncedSelectedPlatforms,
  );
  const canSelectPlatforms = Boolean(canEditProject && hasRequiredContent);
  const canSave = Boolean(
    projectId &&
    canEditProject &&
    hasRequiredContent &&
    selectedPlatforms.length > 0,
  );
  const canOpenXPostIntent = Boolean(canPublishProject && hasRequiredContent);

  useEffect(() => {
    if (!projectId) {
      setProjectRole(null);
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
        setProjectRole(project.role);
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
        setProjectRole(null);
        setContent(emptyContentValue);
        setSelectedPlatforms([]);
        setPrepublishDrafts({});
        setLoadedProjectId(targetProjectId);
        toast.error(t("project.loadFailed"), {
          description:
            requestError instanceof Error
              ? requestError.message
              : t("common.retryLater"),
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

  const openPublishPanel = () => {
    publishBarRef.current?.scrollIntoView({
      behavior: "smooth",
      block: "end",
    });
  };

  const workflow = useContentPublishWorkflow({
    automaticPublishPlatforms,
    canPublish,
    content,
    hasBodyContent,
    navigateToProject: (targetProjectId) =>
      router.replace(`/dashboard/content/${targetProjectId}`),
    prepublishDrafts,
    projectId,
    selectedPlatforms,
    setIsOpeningXPostIntent,
    setIsSaving,
    setIsSyncingPrepublish,
    setPrepublishDrafts,
    setSelectedPlatforms,
    t,
    title,
  });

  const editor = {
    canEdit: canEditProject,
    content,
    setContent: (nextContent: ContentValue) => {
      setContent(nextContent);
      setPrepublishDrafts({});
    },
    setTitle: (nextTitle: string) => {
      setTitle(nextTitle);
      setPrepublishDrafts({});
    },
    title,
  };

  return {
    editor,
    header: {
      canSave,
      isSaving,
      mode: projectId ? ("edit" as const) : ("create" as const),
      onSave: projectId ? workflow.save : undefined,
      projectId,
      projectRole,
    },
    isLoading: isPageLoading,
    openPublishPanel,
    prepublish: {
      canEdit: canEditProject,
      content,
      drafts: prepublishDrafts,
      isSyncing: isSyncingPrepublish,
      onDraftChange: workflow.updatePrepublishDraft,
      onSync: workflow.syncPrepublish,
      projectId,
      title,
    },
    publishBarRef,
    publishing: {
      canOpenXPostIntent,
      canPublish,
      canSelectPlatforms,
      closeDouyinPublishSession: workflow.closeDouyinPublishSession,
      completeDouyinPublishSession: workflow.completeDouyinPublishSession,
      douyinBrowserSession: workflow.douyinBrowserSession,
      isOpeningXPostIntent,
      isPublishing: workflow.isPublishing,
      onOpenDouyinPublishSession: workflow.openDouyinPublishSession,
      onOpenXPostIntent: workflow.openXPostIntent,
      onPublish: workflow.publish,
      onSelectedPlatformsChange: setSelectedPlatforms,
      selectedPlatforms,
    },
  };
}

"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import {
  getPlatformDefaultLabel,
  PLATFORM_TABS,
} from "@/lib/content/platforms";
import { emptyContentValue, type ContentValue } from "@/lib/content/types";
import {
  cancelBrowserSession,
  createDashboardProject,
  getDashboardProject,
  getProjectPublications,
  publishProject,
  saveDashboardProjectContent,
  saveDashboardProjectPlatforms,
  startDouyinPublishSession,
  syncProjectPrepublish,
  updateDashboardProject,
  waitForProjectPublications,
  type AdaptedContent,
  type BrowserSessionStatus,
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

function getBrowserStreamBackendOrigin() {
  const configuredOrigin =
    process.env.NEXT_PUBLIC_BROWSER_STREAM_BASE_URL?.replace(/\/$/, "");
  if (configuredOrigin) {
    return configuredOrigin;
  }

  if (typeof window === "undefined" || process.env.NODE_ENV !== "development") {
    return "";
  }

  const { hostname, port, protocol } = window.location;
  if (
    (hostname === "localhost" || hostname === "127.0.0.1") &&
    port === "3000"
  ) {
    return `${protocol}//${hostname}:8080`;
  }

  return "";
}

function resolveBrowserStreamURL(streamURL: string) {
  if (/^https?:\/\//i.test(streamURL)) {
    return streamURL;
  }

  const backendOrigin = getBrowserStreamBackendOrigin();
  if (!backendOrigin) {
    return streamURL;
  }

  return new URL(streamURL, backendOrigin).toString();
}

type DouyinBrowserSessionState = {
  completing: boolean;
  error?: string;
  expiresAt?: string;
  sessionId: string;
  status: BrowserSessionStatus;
  streamURL?: string;
};

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
  const canPublish = Boolean(
    projectId &&
    hasRequiredContent &&
    automaticPublishPlatforms.length > 0 &&
    hasSyncedSelectedPlatforms,
  );
  const canSelectPlatforms = hasRequiredContent;
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

  const getSelectedPlatformLabels = (platforms: PublishPlatform[]) =>
    platforms.map((platform) => {
      const item = PLATFORM_TABS.find((i) => i.value === platform);
      return item
        ? t(item.label, { defaultValue: item.defaultLabel })
        : platform;
    });

  const openPublishPanel = () => {
    publishBarRef.current?.scrollIntoView({
      behavior: "smooth",
      block: "end",
    });
  };

  const validateContentFields = () => {
    if (!title.trim() || !hasBodyContent) {
      toast.error(t("project.incompleteTitle"), {
        description: t("project.incompleteDesc"),
      });
      return false;
    }

    return true;
  };

  const validatePlatforms = (platforms: PublishPlatform[]) => {
    if (platforms.length === 0) {
      toast.error(t("project.selectPlatformTitle"), {
        description: t("project.selectPlatformDesc"),
      });
      return false;
    }

    return true;
  };

  const validateContent = (
    platforms: PublishPlatform[] = selectedPlatforms,
  ) => {
    if (!validatePlatforms(platforms)) {
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

  const buildProjectContentInput = () => {
    const input = buildProjectInput();
    return {
      cover_image_url: input.cover_image_url,
      source_content: input.source_content,
      summary: input.summary,
      title: input.title,
    };
  };

  const saveOrCreateProjectForManualPlatform = async (
    platform: PublishPlatform,
  ) => {
    const platforms: PublishPlatform[] = selectedPlatforms.includes(platform)
      ? selectedPlatforms
      : [...selectedPlatforms, platform];
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
      toast.success(t("project.saveSuccess"));
    } catch (requestError) {
      toast.error(t("project.saveFailed"), {
        description:
          requestError instanceof Error
            ? requestError.message
            : t("common.retryLater"),
      });
    } finally {
      setIsSaving(false);
    }
  };

  const syncPrepublish = async (
    platforms: PublishPlatform[] = selectedPlatforms,
  ) => {
    if (!validateContent(platforms)) {
      return;
    }

    setIsSyncingPrepublish(true);
    try {
      const targetProject = projectId
        ? await updateDashboardProject(projectId, buildProjectInput(platforms))
        : await createDashboardProject(buildProjectInput(platforms));
      const publications = await syncProjectPrepublish(targetProject.id, {
        platforms,
      });

      setSelectedPlatforms(platforms);
      setPrepublishDrafts(draftsFromPublications(publications));
      toast.success(t("project.syncSuccess"), {
        description: t("project.syncDesc"),
      });
      if (!projectId) {
        router.replace(`/dashboard/content/${targetProject.id}`);
      }
    } catch (requestError) {
      toast.error(t("project.syncFailed"), {
        description:
          requestError instanceof Error
            ? requestError.message
            : t("common.retryLater"),
      });
    } finally {
      setIsSyncingPrepublish(false);
    }
  };

  const updatePrepublishDraft = (
    platform: PublishPlatform,
    draft: PrepublishDraft,
  ) => {
    setPrepublishDrafts({
      ...prepublishDrafts,
      [platform]: draft,
    });
  };

  const [isPublishing, setIsPublishing] = useState(false);
  const [douyinBrowserSession, setDouyinBrowserSession] =
    useState<DouyinBrowserSessionState | null>(null);

  const publishExistingProject = async () => {
    if (!projectId) {
      return;
    }

    await saveDashboardProjectContent(projectId, buildProjectContentInput());
    await saveDashboardProjectPlatforms(projectId, {
      platforms: automaticPublishPlatforms,
    });

    const results = await Promise.allSettled(
      automaticPublishPlatforms.map(async (platform) => {
        const result = await publishProject(projectId, platform);
        if (result.status === "failed" || result.status === "error") {
          throw new Error(
            result.error_message ||
              `${getPlatformDefaultLabel(platform)} ${t("publish.failed")}`,
          );
        }
        return {
          platform,
          status: result.status,
        };
      }),
    );

    const succeeded: PublishPlatform[] = [];
    const failed: { message: string; platform: PublishPlatform }[] = [];
    const pendingPlatforms: PublishPlatform[] = [];

    results.forEach((result, index) => {
      const platform = automaticPublishPlatforms[index];
      if (result.status === "fulfilled") {
        if (result.value.status === "publishing") {
          pendingPlatforms.push(platform);
          return;
        }
        succeeded.push(result.value.platform);
        return;
      }

      failed.push({
        message:
          result.reason instanceof Error
            ? result.reason.message
            : t("common.retryLater"),
        platform,
      });
    });

    if (pendingPlatforms.length > 0) {
      const finalPublications = await waitForProjectPublications(
        projectId,
        automaticPublishPlatforms,
      );
      const finalPublicationMap = new Map(
        finalPublications.items.map((publication) => [
          publication.platform,
          publication,
        ]),
      );

      pendingPlatforms.forEach((platform) => {
        const publication = finalPublicationMap.get(platform);
        if (!publication) {
          failed.push({
            message: t("publish.statusMissing", {
              platform: getPlatformDefaultLabel(platform),
            }),
            platform,
          });
          return;
        }

        if (publication.status === "published") {
          succeeded.push(platform);
          return;
        }

        failed.push({
          message:
            publication.error_message ||
            `${getPlatformDefaultLabel(platform)} ${t("publish.failed")}`,
          platform,
        });
      });
    }

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
      const targetProject = await saveOrCreateProjectForManualPlatform("x");
      const result = await publishProject(targetProject.id, "x", {
        mode: "manual",
      });
      if (!result.publish_url) {
        throw new Error(t("publish.xLinkMissing"));
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
          toast.error(t("publish.popupBlocked"), {
            description: t("publish.xLinkCopied"),
          });
          return;
        }
      }

      toast.success(t("publish.xOpened"), {
        description: t("publish.xConfirmHint"),
      });
      if (targetProject.isNew) {
        router.replace(`/dashboard/content/${targetProject.id}`);
      }
    } catch (requestError) {
      popup?.close();
      toast.error(t("publish.xOpenFailed"), {
        description:
          requestError instanceof Error
            ? requestError.message
            : t("common.retryLater"),
      });
    } finally {
      setIsOpeningXPostIntent(false);
    }
  };

  const openDouyinPublishSession = async () => {
    if (!validateContentFields()) {
      return;
    }

    setIsOpeningXPostIntent(true);
    setDouyinBrowserSession(null);
    try {
      const targetProject =
        await saveOrCreateProjectForManualPlatform("douyin");
      const platforms = Array.from(
        new Set<PublishPlatform>([...selectedPlatforms, "douyin"]),
      );
      await saveDashboardProjectPlatforms(targetProject.id, { platforms });
      const publications = await syncProjectPrepublish(targetProject.id, {
        platforms: ["douyin"],
      });
      setSelectedPlatforms(platforms);
      setPrepublishDrafts({
        ...prepublishDrafts,
        ...draftsFromPublications(publications),
      });

      const session = await startDouyinPublishSession(targetProject.id);
      setDouyinBrowserSession({
        completing: false,
        expiresAt: session.expires_at,
        sessionId: session.session_id,
        status: session.status,
        streamURL: resolveBrowserStreamURL(session.stream_url),
      });
      toast.success(t("publish.douyinSessionOpened"));
      if (targetProject.isNew) {
        router.replace(`/dashboard/content/${targetProject.id}`);
      }
    } catch (requestError) {
      toast.error(t("publish.douyinOpenFailed"), {
        description:
          requestError instanceof Error
            ? requestError.message
            : t("common.retryLater"),
      });
    } finally {
      setIsOpeningXPostIntent(false);
    }
  };

  const closeDouyinPublishSession = async () => {
    const sessionId = douyinBrowserSession?.sessionId;
    setDouyinBrowserSession(null);
    if (!sessionId) {
      return;
    }
    try {
      await cancelBrowserSession(sessionId);
    } catch (requestError) {
      toast.error(t("publish.douyinCloseFailed"), {
        description:
          requestError instanceof Error
            ? requestError.message
            : t("common.retryLater"),
      });
    }
  };

  const completeDouyinPublishSession = () => {
    toast.success(t("publish.douyinReviewHint"));
  };

  const publish = async () => {
    if (!validateContent(automaticPublishPlatforms)) {
      return;
    }
    if (!canPublish) {
      toast.error(t("publish.syncRequiredTitle"), {
        description: t("publish.syncRequiredDesc"),
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
          result.succeeded.length > 0
            ? t("publish.partialFailed")
            : t("publish.failed"),
          {
            description: result.failed[0].message,
          },
        );
        return;
      }

      toast.success(
        projectId
          ? t("publish.editAndPublishSuccess")
          : t("publish.publishSuccess"),
        {
          description: t("publish.publishedTo", {
            platforms: getSelectedPlatformLabels(result.succeeded).join(
              t("common.separator", { defaultValue: ", " }),
            ),
          }),
        },
      );
    } catch (requestError) {
      toast.error(t("publish.requestFailed"), {
        description:
          requestError instanceof Error
            ? requestError.message
            : t("common.retryLater"),
      });
    } finally {
      setIsPublishing(false);
    }
  };

  return {
    canSave,
    canOpenXPostIntent,
    canPublish,
    canSelectPlatforms,
    closeDouyinPublishSession: () => void closeDouyinPublishSession(),
    content,
    completeDouyinPublishSession,
    douyinBrowserSession,
    isEditing: Boolean(projectId),
    isLoading: isPageLoading,
    isOpeningXPostIntent,
    isPublishing,
    isSaving,
    isSyncingPrepublish,
    openPublishPanel,
    openDouyinPublishSession: () => void openDouyinPublishSession(),
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
    updatePrepublishDraft,
  };
}

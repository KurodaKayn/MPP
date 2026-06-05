"use client";

import { useRef, useState } from "react";
import { toast } from "sonner";
import {
  getPlatformDefaultLabel,
  PLATFORM_TABS,
} from "@/lib/content/platforms";
import type { ContentValue } from "@/lib/content/types";
import {
  cancelBrowserSession,
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
  type ProjectListItem,
  type ProjectPublications,
} from "@/lib/dashboard/api";
import {
  createPlatformPublishIdempotencyKey,
  createPublishAttemptKey,
  type PublishPlatform,
} from "../_lib/publish-content";
import { type PrepublishDraft } from "../_stores/content-page-store";

type TranslationFn = (key: string, options?: Record<string, unknown>) => string;

export type DouyinBrowserSessionState = {
  completing: boolean;
  error?: string;
  expiresAt?: string;
  sessionId: string;
  status: BrowserSessionStatus;
  streamURL?: string;
};

type ContentPublishWorkflowOptions = {
  automaticPublishPlatforms: PublishPlatform[];
  canPublish: boolean;
  content: ContentValue;
  createProject: (input: CreateProjectInput) => Promise<ProjectListItem>;
  hasBodyContent: boolean;
  navigateToProject: (projectId: string) => void;
  prepublishDrafts: Partial<Record<PublishPlatform, PrepublishDraft>>;
  projectId?: string;
  selectedPlatforms: PublishPlatform[];
  setIsOpeningXPostIntent: (isOpeningXPostIntent: boolean) => void;
  setIsSaving: (isSaving: boolean) => void;
  setIsSyncingPrepublish: (isSyncingPrepublish: boolean) => void;
  setPrepublishDrafts: (
    prepublishDrafts: Partial<Record<PublishPlatform, PrepublishDraft>>,
  ) => void;
  setSelectedPlatforms: (selectedPlatforms: PublishPlatform[]) => void;
  t: TranslationFn;
  title: string;
};

type PublishAttemptState = {
  key: string;
  signature: string;
};

class PublishStatusError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "PublishStatusError";
  }
}

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

function isPrepublishFormat(
  format: AdaptedContent["format"],
): format is PrepublishDraft["format"] {
  return format === "html" || format === "markdown" || format === "text";
}

export function draftsFromPublications(publications: ProjectPublications) {
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

export function useContentPublishWorkflow({
  automaticPublishPlatforms,
  canPublish,
  content,
  createProject,
  hasBodyContent,
  navigateToProject,
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
}: ContentPublishWorkflowOptions) {
  const [isPublishing, setIsPublishing] = useState(false);
  const [douyinBrowserSession, setDouyinBrowserSession] =
    useState<DouyinBrowserSessionState | null>(null);
  const publishAttemptRef = useRef<PublishAttemptState | null>(null);

  const getSelectedPlatformLabels = (platforms: PublishPlatform[]) =>
    platforms.map((platform) => {
      const item = PLATFORM_TABS.find((i) => i.value === platform);
      return item
        ? t(item.label, { defaultValue: item.defaultLabel })
        : platform;
    });

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

  const buildPublishAttemptSignature = () =>
    JSON.stringify({
      content: buildProjectContentInput(),
      drafts: automaticPublishPlatforms.map((platform) => ({
        draft: prepublishDrafts[platform],
        platform,
      })),
      platforms: automaticPublishPlatforms,
      projectId,
    });

  const getPublishAttemptKey = () => {
    if (!projectId) {
      return "";
    }

    const signature = buildPublishAttemptSignature();
    if (publishAttemptRef.current?.signature !== signature) {
      publishAttemptRef.current = {
        key: createPublishAttemptKey(projectId, automaticPublishPlatforms),
        signature,
      };
    }

    return publishAttemptRef.current.key;
  };

  const clearPublishAttempt = () => {
    publishAttemptRef.current = null;
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

    const project = await createProject(input);
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
        : await createProject(buildProjectInput(platforms));
      const publications = await syncProjectPrepublish(targetProject.id, {
        platforms,
      });

      setSelectedPlatforms(platforms);
      setPrepublishDrafts(draftsFromPublications(publications));
      toast.success(t("project.syncSuccess"), {
        description: t("project.syncDesc"),
      });
      if (!projectId) {
        navigateToProject(targetProject.id);
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

  const publishExistingProject = async () => {
    if (!projectId) {
      return;
    }

    await saveDashboardProjectContent(projectId, buildProjectContentInput());
    await saveDashboardProjectPlatforms(projectId, {
      platforms: automaticPublishPlatforms,
    });

    const publishAttemptKey = getPublishAttemptKey();
    const results = await Promise.allSettled(
      automaticPublishPlatforms.map(async (platform) => {
        const result = await publishProject(projectId, platform, {
          idempotencyKey: createPlatformPublishIdempotencyKey(
            publishAttemptKey,
            platform,
          ),
        });
        if (result.status === "failed" || result.status === "error") {
          throw new PublishStatusError(
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
    let retryable = false;

    results.forEach((result, index) => {
      const platform = automaticPublishPlatforms[index];
      if (result.status === "fulfilled") {
        if (
          result.value.status === "queued" ||
          result.value.status === "publishing"
        ) {
          pendingPlatforms.push(platform);
          return;
        }
        succeeded.push(result.value.platform);
        return;
      }

      if (!(result.reason instanceof PublishStatusError)) {
        retryable = true;
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

        if (publication.status === "succeeded") {
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

    return { failed, retryable, succeeded };
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
        navigateToProject(targetProject.id);
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
        navigateToProject(targetProject.id);
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

      if (!result.retryable) {
        clearPublishAttempt();
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
    closeDouyinPublishSession: () => void closeDouyinPublishSession(),
    completeDouyinPublishSession,
    douyinBrowserSession,
    isPublishing,
    openDouyinPublishSession: () => void openDouyinPublishSession(),
    openXPostIntent: () => void openXPostIntent(),
    publish: () => void publish(),
    save: () => void save(),
    syncPrepublish,
    updatePrepublishDraft,
  };
}

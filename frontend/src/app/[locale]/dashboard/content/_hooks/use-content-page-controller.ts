"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { hasPendingLocalMedia } from "@/components/dashboard/content/editor/content-editor-media";
import { PLATFORM_TABS } from "@/lib/content/platforms";
import { emptyContentValue, type ContentValue } from "@/lib/content/types";
import { canCreateWorkspaceProject } from "../../_hooks/use-dashboard-workspace-selection";
import {
  cancelScheduledPublication,
  createDashboardProject,
  createWorkspaceProject,
  getBrandProfiles,
  getDashboardProject,
  getContentTemplates,
  getProjectPublications,
  getWorkspacePublicationCalendar,
  getWorkspaceBrandProfiles,
  getWorkspaceContentTemplates,
  retryScheduledPublication,
  scheduleProjectPublication,
} from "@/lib/dashboard/api";
import type {
  BrandProfile,
  ContentTemplate,
  ProjectPermissionSource,
  ProjectRole,
  ScheduledPublication,
  Workspace,
} from "@/lib/dashboard/api";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import { type PublishPlatform } from "../_lib/publish-content";
import { useContentPageStore } from "../_stores/content-page-store";
import {
  draftsFromPublications,
  useContentPublishWorkflow,
} from "../_workflows/use-content-publish-workflow";

type WorkspaceProjectContext = {
  requiresWorkspace?: boolean;
  selectedWorkspace?: Workspace | null;
};

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

export function useContentPageController(
  projectId?: string,
  workspaceContext: WorkspaceProjectContext = {},
) {
  const { requiresWorkspace = false, selectedWorkspace = null } =
    workspaceContext;
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
  const router = useRouter();
  const { t } = useTranslation(locale, "common");
  const [projectRole, setProjectRole] = useState<ProjectRole | null>(null);
  const [contentTemplates, setContentTemplates] = useState<ContentTemplate[]>(
    [],
  );
  const [brandProfiles, setBrandProfiles] = useState<BrandProfile[]>([]);
  const [selectedTemplateId, setSelectedTemplateId] = useState("");
  const [selectedBrandProfileId, setSelectedBrandProfileId] = useState("");
  const [permissionSources, setPermissionSources] = useState<
    ProjectPermissionSource[]
  >([]);
  const [projectWorkspaceId, setProjectWorkspaceId] = useState("");
  const [scheduledPublications, setScheduledPublications] = useState<
    ScheduledPublication[]
  >([]);
  const [isSchedulingPublication, setIsSchedulingPublication] = useState(false);
  const [busyScheduleId, setBusyScheduleId] = useState("");
  const [setupError, setSetupError] = useState("");
  const [isSetupLoading, setIsSetupLoading] = useState(false);
  const publishBarRef = useRef<HTMLDivElement>(null);
  const isRouteContentLoaded = projectId
    ? loadedProjectId === projectId
    : loadedProjectId === null;
  const isPageLoading = isLoading || !isRouteContentLoaded;
  const hasBodyContent = Boolean(content.text.trim() || content.firstImageSrc);
  const hasRequiredContent = Boolean(
    !isPageLoading && title.trim() && hasBodyContent,
  );
  const hasUnsavedLocalMedia = hasPendingLocalMedia(content.html);
  const isSaveBlockedAction = isSaving || hasUnsavedLocalMedia;
  const automaticPublishPlatforms = selectedPlatforms.filter(
    (platform) => platform !== "douyin",
  );
  const hasSyncedSelectedPlatforms = automaticPublishPlatforms.every(
    (platform) => {
      const draft = prepublishDrafts[platform];
      return Boolean(
        draft &&
        !draft.syncRequired &&
        (draft.draftStatus === undefined || draft.draftStatus === "ready"),
      );
    },
  );
  const canEditProject = !projectId || projectRole !== "viewer";
  const canPublishProject = !projectId || projectRole === "owner";
  const canCreateInWorkspace = Boolean(
    projectId ||
    (!requiresWorkspace && !selectedWorkspace) ||
    (selectedWorkspace && canCreateWorkspaceProject(selectedWorkspace.role)),
  );
  const canPublish = Boolean(
    projectId &&
    canPublishProject &&
    hasRequiredContent &&
    !isSaveBlockedAction &&
    automaticPublishPlatforms.length > 0 &&
    hasSyncedSelectedPlatforms,
  );
  const canSelectPlatforms = Boolean(
    canEditProject && canCreateInWorkspace && hasRequiredContent && !isSaving,
  );
  const canSave = Boolean(
    projectId &&
    canEditProject &&
    hasRequiredContent &&
    selectedPlatforms.length > 0,
  );
  const canOpenXPostIntent = Boolean(
    canPublishProject &&
    canCreateInWorkspace &&
    hasRequiredContent &&
    !isSaveBlockedAction,
  );

  const guardSaveBlockedAction = () => {
    if (hasUnsavedLocalMedia) {
      toast.error(t("project.savePendingMediaTitle"), {
        description: t("project.savePendingMediaDesc"),
      });
      return true;
    }

    if (isSaving) {
      toast.error(t("project.saveInProgressTitle"), {
        description: t("project.saveInProgressDesc"),
      });
      return true;
    }

    return false;
  };

  useEffect(() => {
    if (!projectId) {
      setProjectRole(null);
      setProjectWorkspaceId("");
      setScheduledPublications([]);
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
        setProjectWorkspaceId(project.workspace_id ?? "");
        setSelectedTemplateId(project.template_id ?? "");
        setSelectedBrandProfileId(project.brand_profile_id ?? "");
        setPermissionSources(project.permission_sources ?? []);
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
        setProjectWorkspaceId("");
        setScheduledPublications([]);
        setSelectedTemplateId("");
        setSelectedBrandProfileId("");
        setPermissionSources([]);
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

  useEffect(() => {
    if (!projectId || !projectWorkspaceId) {
      setScheduledPublications([]);
      return;
    }

    let cancelled = false;
    const from = new Date(Date.now() - 24 * 60 * 60 * 1000);
    const to = new Date(Date.now() + 30 * 24 * 60 * 60 * 1000);

    async function loadSchedules() {
      try {
        const calendar = await getWorkspacePublicationCalendar(
          projectWorkspaceId,
          from.toISOString(),
          to.toISOString(),
        );
        if (cancelled) {
          return;
        }
        setScheduledPublications(
          calendar.items.filter((item) => item.project_id === projectId),
        );
      } catch {
        if (!cancelled) {
          setScheduledPublications([]);
        }
      }
    }

    void loadSchedules();

    return () => {
      cancelled = true;
    };
  }, [projectId, projectWorkspaceId]);

  useEffect(() => {
    if (projectId) {
      return;
    }

    const workspaceId = selectedWorkspace?.id;
    if (requiresWorkspace && !workspaceId) {
      setContentTemplates([]);
      setBrandProfiles([]);
      setSelectedTemplateId("");
      setSelectedBrandProfileId("");
      setSetupError("");
      return;
    }

    let cancelled = false;
    async function loadSetupOptions() {
      setIsSetupLoading(true);
      setSetupError("");
      try {
        const [templatesResp, profilesResp] = workspaceId
          ? await Promise.all([
              getWorkspaceContentTemplates(workspaceId),
              getWorkspaceBrandProfiles(workspaceId),
            ])
          : await Promise.all([getContentTemplates(), getBrandProfiles()]);
        if (cancelled) {
          return;
        }

        setContentTemplates(templatesResp.items);
        setBrandProfiles(profilesResp.items);
        setSelectedTemplateId((current) =>
          current && templatesResp.items.some((item) => item.id === current)
            ? current
            : "",
        );
        setSelectedBrandProfileId((current) =>
          current && profilesResp.items.some((item) => item.id === current)
            ? current
            : "",
        );
      } catch (requestError) {
        if (cancelled) {
          return;
        }
        setContentTemplates([]);
        setBrandProfiles([]);
        setSetupError(
          requestError instanceof Error
            ? requestError.message
            : t("common.retryLater"),
        );
      } finally {
        if (!cancelled) {
          setIsSetupLoading(false);
        }
      }
    }

    void loadSetupOptions();

    return () => {
      cancelled = true;
    };
  }, [projectId, requiresWorkspace, selectedWorkspace?.id]);

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
    createProject: (input) => {
      if (selectedWorkspace) {
        return createWorkspaceProject(selectedWorkspace.id, input);
      }
      if (requiresWorkspace) {
        throw new Error(t("project.workspaceRequired"));
      }

      return createDashboardProject(input);
    },
    hasBodyContent,
    navigateToProject: (targetProjectId) => {
      router.replace(
        `/${locale}/dashboard/content?projectId=${encodeURIComponent(targetProjectId)}`,
      );
    },
    prepublishDrafts,
    projectId,
    selectedBrandProfileId,
    selectedPlatforms,
    selectedTemplateId,
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
    permissionSources,
    setContent: (nextContent: ContentValue) => {
      setContent(nextContent);
      setPrepublishDrafts({});
    },
    setTitle: (nextTitle: string) => {
      setTitle(nextTitle);
      setPrepublishDrafts({});
    },
    restoreVersionContent: (project: {
      title: string;
      source_content: string;
    }) => {
      setTitle(project.title);
      setContent(contentValueFromSource(project.source_content));
      setPrepublishDrafts({});
    },
    title,
  };

  const applyTemplate = (templateId: string) => {
    if (templateId === selectedTemplateId) {
      return;
    }
    if (!templateId) {
      setSelectedTemplateId("");
      return;
    }

    const template = contentTemplates.find((item) => item.id === templateId);
    if (!template) {
      return;
    }
    const hasExistingDraft = Boolean(
      title.trim() || content.text.trim() || content.firstImageSrc,
    );
    if (
      hasExistingDraft &&
      !window.confirm(t("content.setup.replaceConfirm"))
    ) {
      return;
    }

    setSelectedTemplateId(templateId);
    setTitle(template.title_template);
    setContent(contentValueFromSource(template.source_template));
    setSelectedPlatforms(template.default_platforms.filter(isPublishPlatform));
    setPrepublishDrafts({});
  };

  const upsertScheduledPublication = (next: ScheduledPublication) => {
    setScheduledPublications((current) => {
      const existingIndex = current.findIndex((item) => item.id === next.id);
      if (existingIndex === -1) {
        return [...current, next].sort(
          (left, right) =>
            new Date(left.scheduled_at).getTime() -
            new Date(right.scheduled_at).getTime(),
        );
      }
      return current.map((item) => (item.id === next.id ? next : item));
    });
    if (next.workspace_id) {
      setProjectWorkspaceId(next.workspace_id);
    }
  };

  const schedulePublication = async (
    platform: PublishPlatform,
    scheduledAt: string,
  ) => {
    if (!projectId) {
      return;
    }
    const scheduledDate = new Date(scheduledAt);
    if (Number.isNaN(scheduledDate.getTime())) {
      toast.error(
        t("publish.scheduleInvalid", {
          defaultValue: "Choose a valid publish time.",
        }),
      );
      return;
    }

    setIsSchedulingPublication(true);
    try {
      const schedule = await scheduleProjectPublication(projectId, {
        idempotency_key: `${projectId}:${platform}:schedule:${scheduledDate.toISOString()}`,
        platform,
        scheduled_at: scheduledDate.toISOString(),
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC",
      });
      upsertScheduledPublication(schedule);
      toast.success(
        t("publish.scheduleCreated", { defaultValue: "Publish scheduled." }),
      );
    } catch (requestError) {
      toast.error(
        t("publish.scheduleFailed", {
          defaultValue: "Could not schedule publish.",
        }),
        {
          description:
            requestError instanceof Error
              ? requestError.message
              : t("common.retryLater"),
        },
      );
    } finally {
      setIsSchedulingPublication(false);
    }
  };

  const cancelSchedule = async (scheduleId: string) => {
    if (!projectId) {
      return;
    }
    setBusyScheduleId(scheduleId);
    try {
      const schedule = await cancelScheduledPublication(projectId, scheduleId);
      upsertScheduledPublication(schedule);
      toast.success(
        t("publish.scheduleCancelled", { defaultValue: "Schedule cancelled." }),
      );
    } catch (requestError) {
      toast.error(
        t("publish.cancelScheduleFailed", {
          defaultValue: "Could not cancel schedule.",
        }),
        {
          description:
            requestError instanceof Error
              ? requestError.message
              : t("common.retryLater"),
        },
      );
    } finally {
      setBusyScheduleId("");
    }
  };

  const retrySchedule = async (scheduleId: string) => {
    if (!projectId) {
      return;
    }
    setBusyScheduleId(scheduleId);
    try {
      const schedule = await retryScheduledPublication(projectId, scheduleId);
      upsertScheduledPublication(schedule);
      toast.success(
        t("publish.retryQueued", { defaultValue: "Publish retried." }),
      );
    } catch (requestError) {
      toast.error(
        t("publish.retryFailed", { defaultValue: "Could not retry publish." }),
        {
          description:
            requestError instanceof Error
              ? requestError.message
              : t("common.retryLater"),
        },
      );
    } finally {
      setBusyScheduleId("");
    }
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
      canEdit: Boolean(
        canEditProject && canCreateInWorkspace && !isSaveBlockedAction,
      ),
      content,
      drafts: prepublishDrafts,
      isSyncing: isSyncingPrepublish,
      onDraftChange: workflow.updatePrepublishDraft,
      onSync: (platforms?: PublishPlatform[]) => {
        if (guardSaveBlockedAction()) {
          return;
        }

        return workflow.syncPrepublish(platforms);
      },
      projectId,
      title,
    },
    publishBarRef,
    setup: {
      brandProfiles,
      contentTemplates,
      error: setupError,
      isLoading: isSetupLoading,
      onBrandProfileChange: setSelectedBrandProfileId,
      onTemplateChange: applyTemplate,
      selectedBrandProfileId,
      selectedTemplateId,
    },
    publishing: {
      canOpenXPostIntent,
      canPublish,
      canSelectPlatforms,
      closeDouyinPublishSession: workflow.closeDouyinPublishSession,
      completeDouyinPublishSession: workflow.completeDouyinPublishSession,
      douyinBrowserSession: workflow.douyinBrowserSession,
      isOpeningXPostIntent,
      isPublishing: workflow.isPublishing,
      isSchedulingPublication,
      onOpenDouyinPublishSession: () => {
        if (guardSaveBlockedAction()) {
          return;
        }

        return workflow.openDouyinPublishSession();
      },
      onOpenXPostIntent: () => {
        if (guardSaveBlockedAction()) {
          return;
        }

        return workflow.openXPostIntent();
      },
      onPublish: () => {
        if (guardSaveBlockedAction()) {
          return;
        }

        return workflow.publish();
      },
      onCancelSchedule: cancelSchedule,
      onRetrySchedule: retrySchedule,
      onSchedulePublication: (
        platform: PublishPlatform,
        scheduledAt: string,
      ) => {
        if (guardSaveBlockedAction()) {
          return;
        }

        return schedulePublication(platform, scheduledAt);
      },
      onSelectedPlatformsChange: setSelectedPlatforms,
      busyScheduleId,
      scheduledPublications,
      selectedPlatforms,
    },
  };
}

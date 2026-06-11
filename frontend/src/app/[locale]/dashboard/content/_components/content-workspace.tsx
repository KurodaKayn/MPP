"use client";

import { useAuth } from "@/components/auth/auth-provider";
import { ContentEditor } from "@/components/dashboard/content/content-editor";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useProjectCollabConnection } from "@/features/collab-editor/use-collab-document";
import { useCallback, useRef, useState } from "react";
import { DashboardErrorCard } from "../../_components/dashboard-error-card";
import { WorkspaceSwitcher } from "../../_components/workspace-switcher";
import {
  canCreateWorkspaceProject,
  useDashboardWorkspaceSelection,
} from "../../_hooks/use-dashboard-workspace-selection";
import { cn } from "@/lib/utils";
import { ContentPageHeader } from "./content-page-header";
import { ContentPrepublishPanel } from "./content-prepublish-panel";
import { ContentPublishBar } from "./content-publish-bar";
import { PlatformPreview } from "./platform-preview";
import { ProjectCollaborationPanel } from "./project-collaboration-panel";
import { RemoteBrowserSessionModal } from "../../auth/_components/remote-browser-session-modal";
import { useContentPageController } from "../_hooks/use-content-page-controller";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import type { ContentValue } from "@/lib/content/types";
import {
  type ContentView,
  useContentPageStore,
} from "../_stores/content-page-store";

type ContentWorkspaceProps = {
  projectId?: string;
};

export function ContentWorkspace({ projectId }: ContentWorkspaceProps) {
  const { session } = useAuth();
  const [collabReconnectKey, setCollabReconnectKey] = useState(0);
  const prepareContentForSaveRef = useRef<(() => Promise<ContentValue>) | null>(
    null,
  );
  const workspaceSelection = useDashboardWorkspaceSelection({
    enabled: !projectId,
  });
  const contentPage = useContentPageController(projectId, {
    requiresWorkspace: !projectId,
    selectedWorkspace: workspaceSelection.selectedWorkspace,
  });
  const { editor, header, prepublish, publishing, setup } = contentPage;
  const { contentView, setContentView } = useContentPageStore();
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");
  const { t: tDashboard } = useTranslation(locale, "dashboard");
  const projectCollaboration = useProjectCollabConnection({
    projectId,
    reconnectKey: collabReconnectKey,
    userName: session?.username ?? tDashboard("collab.userFallback"),
  });
  const selectedWorkspace = workspaceSelection.selectedWorkspace;
  const workspaceCanCreate = Boolean(
    projectId ||
    (selectedWorkspace && canCreateWorkspaceProject(selectedWorkspace.role)),
  );
  const handlePrepareContentForSaveChange = useCallback(
    (handler: (() => Promise<ContentValue>) | null) => {
      prepareContentForSaveRef.current = handler;
    },
    [],
  );
  const handleSave = header.onSave
    ? () =>
        header.onSave?.({
          prepareContentForSave: prepareContentForSaveRef.current,
        })
    : undefined;

  const isCollabInitialConnectionPending = Boolean(
    projectId &&
    (!projectCollaboration.provider ||
      !projectCollaboration.user ||
      !projectCollaboration.ydoc) &&
    (projectCollaboration.status === "idle" ||
      projectCollaboration.status === "connecting"),
  );

  if (contentPage.isLoading || isCollabInitialConnectionPending) {
    return (
      <div className="flex flex-col gap-6 pb-4">
        <div className="space-y-2">
          <Skeleton className="h-9 w-40" />
          <Skeleton className="h-5 w-80 max-w-full" />
        </div>
        <div className="space-y-4">
          <Skeleton className="h-9 w-56" />
          <Skeleton className="h-[740px] w-full" />
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6 pb-4">
      <ContentPageHeader
        canSave={header.canSave}
        isSaving={header.isSaving}
        mode={header.mode}
        onOpenPublishPanel={contentPage.openPublishPanel}
        onSave={handleSave}
        projectId={header.projectId}
        projectRole={header.projectRole}
        workspaceControl={
          !projectId ? (
            <WorkspaceSwitcher
              disabled={contentPage.isLoading}
              isLoading={workspaceSelection.isLoading}
              selectedWorkspace={selectedWorkspace}
              workspaces={workspaceSelection.workspaces}
              onWorkspaceChange={workspaceSelection.selectWorkspace}
              onWorkspaceCreate={workspaceSelection.createWorkspace}
              isCreatingWorkspace={workspaceSelection.isCreatingWorkspace}
            />
          ) : undefined
        }
      />

      {workspaceSelection.error ? (
        <DashboardErrorCard
          compact
          title={tDashboard("workspace.error.title")}
          message={workspaceSelection.error}
          retryLabel={tDashboard("workspace.error.retry")}
          onRetry={() => void workspaceSelection.reloadWorkspaces()}
        />
      ) : null}

      {!projectId && !workspaceSelection.isLoading && !workspaceCanCreate ? (
        <div className="rounded-lg border border-dashed bg-muted/20 p-4 text-sm text-muted-foreground">
          {tDashboard("workspace.createProjectDisabled")}
        </div>
      ) : null}

      {!projectId && workspaceCanCreate ? (
        <div className="grid gap-3 rounded-lg border bg-background p-4 sm:grid-cols-2">
          <div className="space-y-2">
            <label
              className="text-sm font-medium"
              htmlFor="content-template"
            >
              {tDashboard("content.setup.template")}
            </label>
            <select
              id="content-template"
              value={setup.selectedTemplateId}
              disabled={setup.isLoading || !setup.contentTemplates.length}
              onChange={(event) => setup.onTemplateChange(event.target.value)}
              className="h-10 w-full rounded-md border bg-background px-3 text-sm outline-none ring-offset-background focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="">
                {setup.isLoading
                  ? tDashboard("content.setup.loading")
                  : tDashboard("content.setup.noTemplate")}
              </option>
              {setup.contentTemplates.map((template) => (
                <option key={template.id} value={template.id}>
                  {template.name}
                </option>
              ))}
            </select>
          </div>

          <div className="space-y-2">
            <label
              className="text-sm font-medium"
              htmlFor="brand-profile"
            >
              {tDashboard("content.setup.brandProfile")}
            </label>
            <select
              id="brand-profile"
              value={setup.selectedBrandProfileId}
              disabled={setup.isLoading || !setup.brandProfiles.length}
              onChange={(event) =>
                setup.onBrandProfileChange(event.target.value)
              }
              className="h-10 w-full rounded-md border bg-background px-3 text-sm outline-none ring-offset-background focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="">
                {setup.isLoading
                  ? tDashboard("content.setup.loading")
                  : tDashboard("content.setup.noBrandProfile")}
              </option>
              {setup.brandProfiles.map((profile) => (
                <option key={profile.id} value={profile.id}>
                  {profile.name}
                </option>
              ))}
            </select>
          </div>

          {setup.error ? (
            <p className="text-sm text-destructive sm:col-span-2">
              {setup.error}
            </p>
          ) : null}
        </div>
      ) : null}

      {contentView === "editor" ? (
        <div className="space-y-4">
          <ContentEditor
            canEdit={editor.canEdit}
            collaboration={projectId ? projectCollaboration : undefined}
            title={editor.title}
            content={editor.content}
            onTitleChange={editor.setTitle}
            onContentChange={editor.setContent}
            onPrepareContentForSaveChange={handlePrepareContentForSaveChange}
            projectId={projectId}
            viewSwitcher={
              <ContentViewSwitcher
                value={contentView}
                onValueChange={setContentView}
              />
            }
          />
          {projectId ? (
            <ProjectCollaborationPanel
              canEdit={editor.canEdit}
              onVersionRestore={(project) => {
                editor.restoreVersionContent(project);
                setCollabReconnectKey((current) => current + 1);
              }}
              permissionSources={editor.permissionSources}
              projectId={projectId}
              projectRole={header.projectRole}
            />
          ) : null}
        </div>
      ) : (
        <div>
          <PlatformPreview
            title={editor.title}
            content={editor.content}
            viewSwitcher={
              <ContentViewSwitcher
                value={contentView}
                onValueChange={setContentView}
              />
            }
          />
        </div>
      )}

      <ContentPrepublishPanel
        canEdit={prepublish.canEdit}
        title={prepublish.title}
        content={prepublish.content}
        drafts={prepublish.drafts}
        isSyncing={prepublish.isSyncing}
        onDraftChange={prepublish.onDraftChange}
        onSync={prepublish.onSync}
        projectId={prepublish.projectId}
      />

      <div ref={contentPage.publishBarRef}>
        <ContentPublishBar
          canOpenXPostIntent={publishing.canOpenXPostIntent}
          canPublish={publishing.canPublish}
          canSelectPlatforms={publishing.canSelectPlatforms}
          busyScheduleId={publishing.busyScheduleId}
          isOpeningXPostIntent={publishing.isOpeningXPostIntent}
          isPublishing={publishing.isPublishing}
          isSchedulingPublication={publishing.isSchedulingPublication}
          scheduledPublications={publishing.scheduledPublications}
          selectedPlatforms={publishing.selectedPlatforms}
          onCancelSchedule={publishing.onCancelSchedule}
          onOpenDouyinPublishSession={publishing.onOpenDouyinPublishSession}
          onOpenXPostIntent={publishing.onOpenXPostIntent}
          onPublish={publishing.onPublish}
          onRetrySchedule={publishing.onRetrySchedule}
          onSchedulePublication={publishing.onSchedulePublication}
          onSelectedPlatformsChange={publishing.onSelectedPlatformsChange}
          publishLabel={
            header.mode === "edit"
              ? t("publish.saveAndPublish")
              : t("publish.buttonLabel")
          }
        />
      </div>
      {publishing.douyinBrowserSession ? (
        <RemoteBrowserSessionModal
          completing={publishing.douyinBrowserSession.completing}
          completeLabel={t("publish.douyinPublishedAction")}
          error={publishing.douyinBrowserSession.error}
          expiresAt={publishing.douyinBrowserSession.expiresAt}
          platformLabel={t("platforms.douyin", { defaultValue: "Douyin" })}
          status={publishing.douyinBrowserSession.status}
          streamURL={publishing.douyinBrowserSession.streamURL}
          onCancel={publishing.closeDouyinPublishSession}
          onComplete={publishing.completeDouyinPublishSession}
        />
      ) : null}
    </div>
  );
}

function ContentViewSwitcher({
  onValueChange,
  value,
}: {
  onValueChange: (value: ContentView) => void;
  value: ContentView;
}) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");

  return (
    <div className="inline-flex rounded-lg border bg-muted p-0.5">
      {[
        ["editor", t("common.edit")],
        ["preview", t("common.preview")],
      ].map(([itemValue, label]) => (
        <Button
          key={itemValue}
          type="button"
          size="sm"
          variant={value === itemValue ? "default" : "ghost"}
          className={cn(
            "h-7 rounded-md px-3 text-xs",
            value !== itemValue && "text-muted-foreground",
          )}
          onClick={() => onValueChange(itemValue as ContentView)}
        >
          {label}
        </Button>
      ))}
    </div>
  );
}

"use client";

import { ContentEditor } from "@/components/dashboard/content/content-editor";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { ContentPageHeader } from "./content-page-header";
import { ContentPrepublishPanel } from "./content-prepublish-panel";
import { ContentPublishBar } from "./content-publish-bar";
import { PlatformPreview } from "./platform-preview";
import { RemoteBrowserSessionModal } from "../../auth/_components/remote-browser-session-modal";
import { useContentPageController } from "../_hooks/use-content-page-controller";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import {
  type ContentView,
  useContentPageStore,
} from "../_stores/content-page-store";

type ContentWorkspaceProps = {
  projectId?: string;
};

export function ContentWorkspace({ projectId }: ContentWorkspaceProps) {
  const contentPage = useContentPageController(projectId);
  const { contentView, setContentView } = useContentPageStore();
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");

  if (contentPage.isLoading) {
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
        canSave={contentPage.canSave}
        isSaving={contentPage.isSaving}
        mode={contentPage.isEditing ? "edit" : "create"}
        onOpenPublishPanel={contentPage.openPublishPanel}
        onSave={contentPage.isEditing ? contentPage.save : undefined}
      />

      {contentView === "editor" ? (
        <div>
          <ContentEditor
            title={contentPage.title}
            content={contentPage.content}
            onTitleChange={contentPage.setTitle}
            onContentChange={contentPage.setContent}
            viewSwitcher={
              <ContentViewSwitcher
                value={contentView}
                onValueChange={setContentView}
              />
            }
          />
        </div>
      ) : (
        <div>
          <PlatformPreview
            title={contentPage.title}
            content={contentPage.content}
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
        title={contentPage.title}
        content={contentPage.content}
        drafts={contentPage.prepublishDrafts}
        isSyncing={contentPage.isSyncingPrepublish}
        onDraftChange={contentPage.updatePrepublishDraft}
        onSync={contentPage.syncPrepublish}
        projectId={projectId}
      />

      <div ref={contentPage.publishBarRef}>
        <ContentPublishBar
          canOpenXPostIntent={contentPage.canOpenXPostIntent}
          canPublish={contentPage.canPublish}
          canSelectPlatforms={contentPage.canSelectPlatforms}
          isOpeningXPostIntent={contentPage.isOpeningXPostIntent}
          isPublishing={contentPage.isPublishing}
          selectedPlatforms={contentPage.selectedPlatforms}
          onOpenDouyinPublishSession={contentPage.openDouyinPublishSession}
          onOpenXPostIntent={contentPage.openXPostIntent}
          onPublish={contentPage.publish}
          onSelectedPlatformsChange={contentPage.setSelectedPlatforms}
          publishLabel={
            contentPage.isEditing
              ? t("publish.saveAndPublish")
              : t("publish.buttonLabel")
          }
        />
      </div>
      {contentPage.douyinBrowserSession ? (
        <RemoteBrowserSessionModal
          completing={contentPage.douyinBrowserSession.completing}
          completeLabel={t("publish.douyinPublishedAction")}
          error={contentPage.douyinBrowserSession.error}
          expiresAt={contentPage.douyinBrowserSession.expiresAt}
          platformLabel={t("platforms.douyin", { defaultValue: "Douyin" })}
          status={contentPage.douyinBrowserSession.status}
          streamURL={contentPage.douyinBrowserSession.streamURL}
          onCancel={contentPage.closeDouyinPublishSession}
          onComplete={contentPage.completeDouyinPublishSession}
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

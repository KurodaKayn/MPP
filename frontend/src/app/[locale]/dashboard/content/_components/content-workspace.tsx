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
  const { editor, header, prepublish, publishing } = contentPage;
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
        canSave={header.canSave}
        isSaving={header.isSaving}
        mode={header.mode}
        onOpenPublishPanel={contentPage.openPublishPanel}
        onSave={header.onSave}
      />

      {contentView === "editor" ? (
        <div>
          <ContentEditor
            title={editor.title}
            content={editor.content}
            onTitleChange={editor.setTitle}
            onContentChange={editor.setContent}
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
          isOpeningXPostIntent={publishing.isOpeningXPostIntent}
          isPublishing={publishing.isPublishing}
          selectedPlatforms={publishing.selectedPlatforms}
          onOpenDouyinPublishSession={publishing.onOpenDouyinPublishSession}
          onOpenXPostIntent={publishing.onOpenXPostIntent}
          onPublish={publishing.onPublish}
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

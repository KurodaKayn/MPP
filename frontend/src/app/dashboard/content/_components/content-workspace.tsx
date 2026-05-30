"use client";

import { ContentEditor } from "@/components/dashboard/content/content-editor";
import { Skeleton } from "@/components/ui/skeleton";
import { ContentPageHeader } from "./content-page-header";
import { ContentPublishBar } from "./content-publish-bar";
import { PlatformPreview } from "./platform-preview";
import { useContentPageController } from "../_hooks/use-content-page-controller";

type ContentWorkspaceProps = {
  projectId?: string;
};

export function ContentWorkspace({ projectId }: ContentWorkspaceProps) {
  const contentPage = useContentPageController(projectId);

  if (contentPage.isLoading) {
    return (
      <div className="flex flex-col gap-6 pb-4">
        <div className="space-y-2">
          <Skeleton className="h-9 w-40" />
          <Skeleton className="h-5 w-80 max-w-full" />
        </div>
        <div className="grid gap-6 xl:grid-cols-[minmax(0,1.35fr)_minmax(340px,0.65fr)]">
          <Skeleton className="h-[740px] w-full" />
          <Skeleton className="h-[560px] w-full" />
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

      <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1.35fr)_minmax(340px,0.65fr)]">
        <ContentEditor
          title={contentPage.title}
          content={contentPage.content}
          onTitleChange={contentPage.setTitle}
          onContentChange={contentPage.setContent}
        />
        <div className="xl:sticky xl:top-6">
          <PlatformPreview
            title={contentPage.title}
            content={contentPage.content}
          />
        </div>
      </div>

      <div ref={contentPage.publishBarRef}>
        <ContentPublishBar
          canPublish={contentPage.canPublish}
          isPublishing={contentPage.isPublishing}
          selectedPlatforms={contentPage.selectedPlatforms}
          onSelectedPlatformsChange={contentPage.setSelectedPlatforms}
          onPublish={contentPage.publish}
          publishLabel={contentPage.isEditing ? "保存并发布" : "一键发布"}
        />
      </div>
    </div>
  );
}

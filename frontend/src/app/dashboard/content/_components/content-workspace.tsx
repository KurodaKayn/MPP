"use client";

import { ContentEditor } from "@/components/dashboard/content/content-editor";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ContentPageHeader } from "./content-page-header";
import { ContentPrepublishPanel } from "./content-prepublish-panel";
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

      <Tabs defaultValue="editor" className="w-full">
        <TabsList>
          <TabsTrigger value="editor">编辑</TabsTrigger>
          <TabsTrigger value="preview">预览</TabsTrigger>
        </TabsList>
        <TabsContent value="editor" className="mt-4">
          <ContentEditor
            title={contentPage.title}
            content={contentPage.content}
            onTitleChange={contentPage.setTitle}
            onContentChange={contentPage.setContent}
          />
        </TabsContent>
        <TabsContent value="preview" className="mt-4">
          <PlatformPreview
            title={contentPage.title}
            content={contentPage.content}
          />
        </TabsContent>
      </Tabs>

      <ContentPrepublishPanel
        title={contentPage.title}
        content={contentPage.content}
        drafts={contentPage.prepublishDrafts}
        isSyncing={contentPage.isSyncingPrepublish}
        selectedPlatforms={contentPage.selectedPlatforms}
        onSelectedPlatformsChange={contentPage.setSelectedPlatforms}
        onSync={contentPage.syncPrepublish}
      />

      <div ref={contentPage.publishBarRef}>
        <ContentPublishBar
          canOpenXPostIntent={contentPage.canOpenXPostIntent}
          canPublish={contentPage.canPublish}
          isOpeningXPostIntent={contentPage.isOpeningXPostIntent}
          isPublishing={contentPage.isPublishing}
          selectedPlatforms={contentPage.selectedPlatforms}
          onOpenXPostIntent={contentPage.openXPostIntent}
          onSelectedPlatformsChange={contentPage.setSelectedPlatforms}
          onPublish={contentPage.publish}
          publishLabel={contentPage.isEditing ? "保存并发布" : "一键发布"}
        />
      </div>
    </div>
  );
}

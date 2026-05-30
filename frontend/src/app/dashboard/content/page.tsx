"use client";

import { ContentEditor } from "@/components/dashboard/content/content-editor";
import { ContentPageHeader } from "./_components/content-page-header";
import { ContentPublishBar } from "./_components/content-publish-bar";
import { PlatformPreview } from "./_components/platform-preview";
import { useContentPageController } from "./_hooks/use-content-page-controller";

export default function ContentPage() {
  const contentPage = useContentPageController();

  return (
    <div className="flex flex-col gap-6 pb-4">
      <ContentPageHeader onOpenPublishPanel={contentPage.openPublishPanel} />

      <div className="grid gap-6 lg:grid-cols-2">
        <ContentEditor
          title={contentPage.title}
          content={contentPage.content}
          onTitleChange={contentPage.setTitle}
          onContentChange={contentPage.setContent}
        />
        <PlatformPreview
          title={contentPage.title}
          content={contentPage.content}
        />
      </div>

      <div ref={contentPage.publishBarRef}>
        <ContentPublishBar
          canPublish={contentPage.canPublish}
          isPublishing={contentPage.isPublishing}
          selectedPlatforms={contentPage.selectedPlatforms}
          onSelectedPlatformsChange={contentPage.setSelectedPlatforms}
          onPublish={contentPage.publish}
        />
      </div>
    </div>
  );
}

import type { PlatformTab } from "@/lib/content/platforms";
import type { ContentValue } from "@/lib/content/types";
import type {
  CreateProjectInput,
  ProjectListItem,
  PublishResult,
} from "@/lib/dashboard/api";
import { waitForProjectPublications } from "@/lib/dashboard/api";

export type PublishPlatform = PlatformTab["value"];

type PublishContentInput = {
  content: ContentValue;
  platforms: PublishPlatform[];
  title: string;
};

type PublishContentDependencies = {
  createProject: (input: CreateProjectInput) => Promise<ProjectListItem>;
  publishProject: (
    projectId: string,
    platform: PublishPlatform,
  ) => Promise<PublishResult>;
  waitForProjectPublications?: typeof waitForProjectPublications;
};

type FailedPublish = {
  message: string;
  platform: PublishPlatform;
};

export type PublishContentResult = {
  failed: FailedPublish[];
  project: ProjectListItem;
  succeeded: PublishPlatform[];
};

export async function publishContentToPlatforms(
  input: PublishContentInput,
  dependencies: PublishContentDependencies,
): Promise<PublishContentResult> {
  const waitForPublications =
    dependencies.waitForProjectPublications ?? waitForProjectPublications;

  const project = await dependencies.createProject({
    cover_image_url: input.content.firstImageSrc || undefined,
    platforms: input.platforms,
    source_content: input.content.html || input.content.text,
    summary: input.content.text,
    title: input.title,
  });

  const results = await Promise.allSettled(
    input.platforms.map(async (platform) => {
      const result = await dependencies.publishProject(project.id, platform);
      if (result.status === "failed" || result.status === "error") {
        throw new Error(result.error_message || `${platform} 发布失败`);
      }
      return {
        platform,
        status: result.status,
      };
    }),
  );

  const succeeded: PublishPlatform[] = [];
  const failed: FailedPublish[] = [];
  const pendingPlatforms: PublishPlatform[] = [];

  results.forEach((result, index) => {
    const platform = input.platforms[index];
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
        result.reason instanceof Error ? result.reason.message : "请稍后重试。",
      platform,
    });
  });

  if (pendingPlatforms.length > 0) {
    const finalPublications = await waitForPublications(
      project.id,
      input.platforms,
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
          message: `${platform} 发布状态未返回`,
          platform,
        });
        return;
      }

      if (publication.status === "published") {
        succeeded.push(platform);
        return;
      }

      failed.push({
        message: publication.error_message || `${platform} 发布失败`,
        platform,
      });
    });
  }

  return {
    failed,
    project,
    succeeded,
  };
}

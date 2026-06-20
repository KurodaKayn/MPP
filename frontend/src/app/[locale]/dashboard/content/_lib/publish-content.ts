import type { PlatformTab } from "@/lib/content/platforms";
import type { ContentValue } from "@/lib/content/types";
import type {
  CreateProjectInput,
  ProjectListItem,
  ProjectPublications,
  PublishProjectOptions,
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
    options?: Pick<PublishProjectOptions, "idempotencyKey">,
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

export type PublishExistingProjectResult = {
  failed: FailedPublish[];
  retryable: boolean;
  succeeded: PublishPlatform[];
};

export function createPublishAttemptKey(
  projectId: string,
  platforms: PublishPlatform[],
) {
  const attemptId = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}`;
  return `${projectId}:${platforms.join(",")}:${attemptId}`;
}

export function createPlatformPublishIdempotencyKey(
  attemptKey: string,
  platform: PublishPlatform,
) {
  return `${attemptKey}:${platform}`;
}

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
  const publishAttemptKey = createPublishAttemptKey(
    project.id,
    input.platforms,
  );

  const result = await publishExistingProjectToPlatforms(
    {
      attemptKey: publishAttemptKey,
      platforms: input.platforms,
      projectId: project.id,
    },
    {
      publishProject: dependencies.publishProject,
      waitForProjectPublications: waitForPublications,
    },
  );

  return {
    failed: result.failed,
    project,
    succeeded: result.succeeded,
  };
}

export async function publishExistingProjectToPlatforms(
  input: {
    attemptKey: string;
    platforms: PublishPlatform[];
    projectId: string;
  },
  dependencies: {
    publishProject: PublishContentDependencies["publishProject"];
    waitForProjectPublications?: (
      projectId: string,
      platforms: PublishPlatform[],
    ) => Promise<ProjectPublications>;
  },
): Promise<PublishExistingProjectResult> {
  const waitForPublications =
    dependencies.waitForProjectPublications ?? waitForProjectPublications;
  const results = await Promise.allSettled(
    input.platforms.map(async (platform) => {
      const result = await dependencies.publishProject(
        input.projectId,
        platform,
        {
          idempotencyKey: createPlatformPublishIdempotencyKey(
            input.attemptKey,
            platform,
          ),
        },
      );
      if (result.status === "failed" || result.status === "error") {
        throw new PublishPlatformError(
          result.error_message || `${platform} publish failed`,
          false,
        );
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
  let retryable = false;

  results.forEach((result, index) => {
    const platform = input.platforms[index];
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

    if (!(result.reason instanceof PublishPlatformError)) {
      retryable = true;
    } else {
      retryable ||= result.reason.retryable;
    }
    failed.push({
      message:
        result.reason instanceof Error
          ? result.reason.message
          : "Please try again later.",
      platform,
    });
  });

  if (pendingPlatforms.length > 0) {
    const finalPublications = await waitForPublications(
      input.projectId,
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
          message: `${platform} publication status not returned`,
          platform,
        });
        return;
      }

      if (publication.status === "succeeded") {
        succeeded.push(platform);
        return;
      }

      failed.push({
        message: publication.error_message || `${platform} publish failed`,
        platform,
      });
    });
  }

  return {
    failed,
    retryable,
    succeeded,
  };
}

class PublishPlatformError extends Error {
  constructor(
    message: string,
    readonly retryable: boolean,
  ) {
    super(message);
    this.name = "PublishPlatformError";
  }
}

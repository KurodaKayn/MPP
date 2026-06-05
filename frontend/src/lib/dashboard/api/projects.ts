import { fetchDashboard, fetchDashboardNoContent } from "./client";
import type {
  AddProjectCollaboratorInput,
  CollabDocumentSession,
  CompleteMediaUploadResult,
  CreateProjectInput,
  CreateMediaUploadInput,
  CreateMediaUploadResult,
  DashboardStats,
  GetProjectPublicationsOptions,
  PaginatedProjects,
  ProjectCollaborator,
  ProjectCollaboratorsResponse,
  ProjectDetail,
  ProjectListItem,
  ProjectPublications,
  PublishProjectOptions,
  PublishResult,
  ResolveMediaAssetsResult,
  SaveProjectContentInput,
  SaveProjectPlatformsInput,
  SyncPrepublishInput,
  StartPublishBrowserSessionResult,
  UpdatePrepublishDraftInput,
  UpdateProjectCollaboratorInput,
  UpdateProjectInput,
  WaitForProjectPublicationsOptions,
} from "./types";

export function getDashboardStats() {
  return fetchDashboard<DashboardStats>("/api/user/dashboard/stats");
}

export function getDashboardProjects(limit = 8) {
  const params = new URLSearchParams({
    page: "1",
    limit: String(limit),
  });

  return fetchDashboard<PaginatedProjects>(
    `/api/user/dashboard/projects?${params.toString()}`,
  );
}

export function createDashboardProject(input: CreateProjectInput) {
  return fetchDashboard<ProjectListItem>("/api/user/dashboard/projects", {
    body: JSON.stringify(input),
    method: "POST",
  });
}

export function getDashboardProject(projectId: string) {
  return fetchDashboard<ProjectDetail>(
    `/api/user/dashboard/projects/${projectId}`,
  );
}

export function updateDashboardProject(
  projectId: string,
  input: UpdateProjectInput,
) {
  return fetchDashboard<ProjectDetail>(
    `/api/user/dashboard/projects/${projectId}`,
    {
      body: JSON.stringify(input),
      method: "PUT",
    },
  );
}

export function saveDashboardProjectContent(
  projectId: string,
  input: SaveProjectContentInput,
) {
  return fetchDashboard<ProjectDetail>(
    `/api/user/dashboard/projects/${projectId}/content`,
    {
      body: JSON.stringify(input),
      method: "PATCH",
    },
  );
}

export function saveDashboardProjectPlatforms(
  projectId: string,
  input: SaveProjectPlatformsInput,
) {
  return fetchDashboard<ProjectDetail>(
    `/api/user/dashboard/projects/${projectId}/platforms`,
    {
      body: JSON.stringify(input),
      method: "PATCH",
    },
  );
}

export function createProjectMediaUpload(
  projectId: string,
  input: CreateMediaUploadInput,
) {
  return fetchDashboard<CreateMediaUploadResult>(
    `/api/user/dashboard/projects/${projectId}/media/uploads`,
    {
      body: JSON.stringify(input),
      method: "POST",
    },
  );
}

export function completeMediaUpload(assetId: string) {
  return fetchDashboard<CompleteMediaUploadResult>(
    `/api/user/dashboard/media/${assetId}/complete`,
    {
      method: "POST",
    },
  );
}

export function resolveMediaAssets(assetIds: string[]) {
  return fetchDashboard<ResolveMediaAssetsResult>(
    "/api/user/dashboard/media/resolve",
    {
      body: JSON.stringify({
        asset_ids: assetIds,
      }),
      method: "POST",
    },
  );
}

export function getProjectCollaborators(projectId: string) {
  return fetchDashboard<ProjectCollaboratorsResponse>(
    `/api/user/dashboard/projects/${projectId}/collaborators`,
  );
}

export function addProjectCollaborator(
  projectId: string,
  input: AddProjectCollaboratorInput,
) {
  return fetchDashboard<ProjectCollaborator>(
    `/api/user/dashboard/projects/${projectId}/collaborators`,
    {
      body: JSON.stringify(input),
      method: "POST",
    },
  );
}

export function updateProjectCollaborator(
  projectId: string,
  userId: string,
  input: UpdateProjectCollaboratorInput,
) {
  return fetchDashboard<ProjectCollaborator>(
    `/api/user/dashboard/projects/${projectId}/collaborators/${userId}`,
    {
      body: JSON.stringify(input),
      method: "PATCH",
    },
  );
}

export function removeProjectCollaborator(projectId: string, userId: string) {
  return fetchDashboardNoContent(
    `/api/user/dashboard/projects/${projectId}/collaborators/${userId}`,
    { method: "DELETE" },
  );
}

export function createProjectCollabSession(projectId: string) {
  return fetchDashboard<CollabDocumentSession>(
    `/api/user/dashboard/projects/${projectId}/collab/session`,
    { method: "POST" },
  );
}

export function getProjectPublications(
  projectId: string,
  options?: GetProjectPublicationsOptions,
) {
  const query = options?.includeContent ? "?include_content=true" : "";

  return fetchDashboard<ProjectPublications>(
    `/api/user/dashboard/projects/${projectId}/publications${query}`,
  );
}

function defaultSleep(ms: number) {
  return new Promise<void>((resolve) => {
    globalThis.setTimeout(resolve, ms);
  });
}

function isPublishingStatus(status: string) {
  return status === "queued" || status === "publishing";
}

export async function waitForProjectPublications(
  projectId: string,
  platforms: string[],
  options: WaitForProjectPublicationsOptions = {},
) {
  const timeoutMs = options.timeoutMs ?? 5 * 60 * 1000;
  const intervalMs = options.intervalMs ?? 2000;
  const fetchProjectPublications =
    options.fetchProjectPublications ?? getProjectPublications;
  const sleep = options.sleep ?? defaultSleep;
  const deadline = Date.now() + timeoutMs;
  const expectedPlatforms = new Set(platforms);

  let latestPublications = await fetchProjectPublications(projectId);

  while (Date.now() <= deadline) {
    const relevantPublications = latestPublications.items.filter((item) =>
      expectedPlatforms.has(item.platform),
    );
    if (
      relevantPublications.length === expectedPlatforms.size &&
      relevantPublications.every(
        (publication) => !isPublishingStatus(publication.status),
      )
    ) {
      return latestPublications;
    }

    await sleep(intervalMs);
    latestPublications = await fetchProjectPublications(projectId);
  }

  throw new Error(
    "Publish request timed out. Please refresh and check again later.",
  );
}

export function syncProjectPrepublish(
  projectId: string,
  input: SyncPrepublishInput,
) {
  return fetchDashboard<ProjectPublications>(
    `/api/user/dashboard/projects/${projectId}/prepublish/sync`,
    {
      body: JSON.stringify({
        actor: input.actor ?? { type: "system" },
        platforms: input.platforms,
      }),
      method: "POST",
    },
  );
}

export function updateProjectPrepublishDraft(
  projectId: string,
  platform: string,
  input: UpdatePrepublishDraftInput,
) {
  return fetchDashboard<ProjectPublications>(
    `/api/user/dashboard/projects/${projectId}/prepublish/${encodeURIComponent(platform)}`,
    {
      body: JSON.stringify(input),
      method: "PUT",
    },
  );
}

export function publishProject(
  projectId: string,
  platform: string,
  options?: PublishProjectOptions,
) {
  const idempotencyKey =
    options?.idempotencyKey ??
    globalThis.crypto?.randomUUID?.() ??
    `${projectId}:${platform}:${Date.now()}`;
  const body = options?.mode
    ? { idempotency_key: idempotencyKey, mode: options.mode, platform }
    : { idempotency_key: idempotencyKey, platform };

  return fetchDashboard<PublishResult>(
    `/api/user/dashboard/projects/${projectId}/publish`,
    {
      body: JSON.stringify(body),
      method: "POST",
    },
  );
}

export function startDouyinPublishSession(projectId: string) {
  return fetchDashboard<StartPublishBrowserSessionResult>(
    `/api/user/dashboard/projects/${projectId}/publish-sessions/douyin`,
    { method: "POST" },
  );
}

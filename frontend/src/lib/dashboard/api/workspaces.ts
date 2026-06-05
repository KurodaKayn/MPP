import { fetchDashboard, fetchDashboardNoContent } from "./client";
import type {
  AddWorkspaceMemberInput,
  CreateProjectInput,
  CreateWorkspaceInput,
  ListWorkspaceProjectsOptions,
  PaginatedProjects,
  ProjectListItem,
  UpdateWorkspaceInput,
  UpdateWorkspaceMemberInput,
  Workspace,
  WorkspaceActivitiesResponse,
  WorkspaceMember,
  WorkspaceMembersResponse,
  WorkspacesResponse,
} from "./types";

export function getWorkspaces() {
  return fetchDashboard<WorkspacesResponse>("/api/workspaces");
}

export function createWorkspace(input: CreateWorkspaceInput) {
  return fetchDashboard<Workspace>("/api/workspaces", {
    body: JSON.stringify(input),
    method: "POST",
  });
}

export function getWorkspaceProjects(
  workspaceId: string,
  options: ListWorkspaceProjectsOptions = {},
) {
  const params = new URLSearchParams();
  if (options.page !== undefined) {
    params.set("page", String(options.page));
  }
  if (options.limit !== undefined) {
    params.set("limit", String(options.limit));
  }
  if (options.status) {
    params.set("status", options.status);
  }
  if (options.platform) {
    params.set("platform", options.platform);
  }

  const query = params.toString();
  return fetchDashboard<PaginatedProjects>(
    `/api/workspaces/${workspaceId}/projects${query ? `?${query}` : ""}`,
  );
}

export function createWorkspaceProject(
  workspaceId: string,
  input: CreateProjectInput,
) {
  return fetchDashboard<ProjectListItem>(
    `/api/workspaces/${workspaceId}/projects`,
    {
      body: JSON.stringify(input),
      method: "POST",
    },
  );
}

export function getWorkspace(workspaceId: string) {
  return fetchDashboard<Workspace>(`/api/workspaces/${workspaceId}`);
}

export function updateWorkspace(
  workspaceId: string,
  input: UpdateWorkspaceInput,
) {
  return fetchDashboard<Workspace>(`/api/workspaces/${workspaceId}`, {
    body: JSON.stringify(input),
    method: "PATCH",
  });
}

export function getWorkspaceMembers(workspaceId: string) {
  return fetchDashboard<WorkspaceMembersResponse>(
    `/api/workspaces/${workspaceId}/members`,
  );
}

export function getWorkspaceActivities(workspaceId: string, limit = 20) {
  const params = new URLSearchParams({
    limit: String(limit),
  });

  return fetchDashboard<WorkspaceActivitiesResponse>(
    `/api/workspaces/${workspaceId}/activity?${params.toString()}`,
  );
}

export function addWorkspaceMember(
  workspaceId: string,
  input: AddWorkspaceMemberInput,
) {
  return fetchDashboard<WorkspaceMember>(
    `/api/workspaces/${workspaceId}/members`,
    {
      body: JSON.stringify(input),
      method: "POST",
    },
  );
}

export function updateWorkspaceMember(
  workspaceId: string,
  userId: string,
  input: UpdateWorkspaceMemberInput,
) {
  return fetchDashboard<WorkspaceMember>(
    `/api/workspaces/${workspaceId}/members/${userId}`,
    {
      body: JSON.stringify(input),
      method: "PATCH",
    },
  );
}

export function removeWorkspaceMember(workspaceId: string, userId: string) {
  return fetchDashboardNoContent(
    `/api/workspaces/${workspaceId}/members/${userId}`,
    { method: "DELETE" },
  );
}

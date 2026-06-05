import { fetchDashboard, fetchDashboardNoContent } from "./client";
import type {
  AddWorkspaceMemberInput,
  CreateWorkspaceInput,
  UpdateWorkspaceInput,
  UpdateWorkspaceMemberInput,
  Workspace,
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

import type {
  AddWorkspaceMemberInput,
  WorkspaceRole,
} from "@/lib/dashboard/api";

export type WorkspaceMemberRole = AddWorkspaceMemberInput["role"];

export const manageableWorkspaceMemberRoles: WorkspaceMemberRole[] = [
  "admin",
  "member",
  "viewer",
];

export function canManageWorkspaceMembers(role?: WorkspaceRole | null) {
  return role === "owner" || role === "admin";
}

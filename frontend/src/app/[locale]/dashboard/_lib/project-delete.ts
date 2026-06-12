import type { ProjectListItem, WorkspaceRole } from "@/lib/dashboard/api";

export type ProjectDeleteSurface = "owned" | "workspace" | "shared-with-me";

type ProjectDeleteOptions = {
  surface: ProjectDeleteSurface;
  workspaceRole?: WorkspaceRole | null;
};

export function canDeleteProjectCard(
  project: Pick<ProjectListItem, "access_source" | "role">,
  options: ProjectDeleteOptions,
) {
  if (options.surface === "shared-with-me") {
    return false;
  }
  if (project.access_source === "owner" || project.role === "owner") {
    return true;
  }
  if (options.surface !== "workspace") {
    return false;
  }
  return options.workspaceRole === "owner" || options.workspaceRole === "admin";
}

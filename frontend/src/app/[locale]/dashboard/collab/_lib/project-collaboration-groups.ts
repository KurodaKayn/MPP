import type { ProjectCollaborator, ProjectListItem } from "@/lib/dashboard/api";

export type ProjectWithCollaborators = {
  collaboratorCount: number;
  collaborators: ProjectCollaborator[];
  project: ProjectListItem;
};

export function getOwnedProjects(projects: ProjectListItem[]) {
  return projects.filter((project) =>
    project.access_source
      ? project.access_source === "owner"
      : project.role === "owner",
  );
}

export function getProjectsSharedWithMe(projects: ProjectListItem[]) {
  return projects.filter((project) => {
    if (project.access_source) {
      return project.access_source === "direct_share";
    }

    return project.role !== "owner" && !project.workspace_id;
  });
}

export function getProjectsSharedByMe(
  projects: Array<{
    collaborators: ProjectCollaborator[];
    project: ProjectListItem;
  }>,
) {
  return projects
    .map(({ collaborators, project }) => ({
      collaboratorCount: collaborators.length,
      collaborators,
      project,
    }))
    .filter((item) => item.collaboratorCount > 0);
}

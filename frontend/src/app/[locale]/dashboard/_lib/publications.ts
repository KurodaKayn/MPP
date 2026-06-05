import type { ProjectListItem } from "@/lib/dashboard/api";

export type PublicationSummary = ProjectListItem["publications"][number];

export function getEnabledPublications(project: ProjectListItem) {
  return project.publications.filter((publication) => publication.enabled);
}

export function getEnabledPlatformCount(projects: ProjectListItem[]) {
  const platforms = new Set<string>();

  for (const project of projects) {
    for (const publication of getEnabledPublications(project)) {
      platforms.add(publication.platform);
    }
  }

  return platforms.size;
}

export function getPublicationTotals(projects: ProjectListItem[]) {
  const enabledPublications = projects.flatMap(getEnabledPublications);

  return {
    failed: enabledPublications.filter((item) => item.status === "failed")
      .length,
    published: enabledPublications.filter((item) => item.status === "succeeded")
      .length,
    total: enabledPublications.length,
  };
}

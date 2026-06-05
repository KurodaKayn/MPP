"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { ArrowRight, Inbox, RefreshCw, Share2, UsersRound } from "lucide-react";
import Link from "next/link";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  getDashboardProjects,
  getProjectCollaborators,
  getWorkspaceProjects,
  type ProjectListItem,
} from "@/lib/dashboard/api";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";

import { DashboardErrorCard } from "../../_components/dashboard-error-card";
import { DashboardStatCard } from "../../_components/dashboard-stat-card";
import { ProjectStatusBadge } from "../../_components/project-status-badge";
import { PlatformIconRow } from "../../_components/publication-platforms";
import { WorkspaceSwitcher } from "../../_components/workspace-switcher";
import { useDashboardWorkspaceSelection } from "../../_hooks/use-dashboard-workspace-selection";
import { formatOptionalDashboardDate } from "../../_lib/formatters";
import { getEnabledPublications } from "../../_lib/publications";
import {
  getOwnedProjects,
  getProjectsSharedByMe,
  getProjectsSharedWithMe,
  type ProjectWithCollaborators,
} from "../_lib/project-collaboration-groups";

function ProjectGridSkeleton() {
  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      {Array.from({ length: 3 }).map((_, index) => (
        <Skeleton key={index} className="h-48 w-full" />
      ))}
    </div>
  );
}

function EmptyState({ message }: { message: string }) {
  return (
    <div className="flex min-h-36 items-center justify-center rounded-lg border border-dashed px-4 text-center text-sm text-muted-foreground">
      {message}
    </div>
  );
}

function CollaborationSection({
  children,
  count,
  description,
  emptyMessage,
  isLoading,
  title,
}: {
  children: React.ReactNode;
  count: number;
  description: string;
  emptyMessage: string;
  isLoading: boolean;
  title: string;
}) {
  return (
    <section className="space-y-3">
      <div className="flex flex-col gap-1 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h3 className="text-lg font-semibold">{title}</h3>
          <p className="text-sm text-muted-foreground">{description}</p>
        </div>
        <Badge variant="outline">{count}</Badge>
      </div>
      {isLoading ? (
        <ProjectGridSkeleton />
      ) : count === 0 ? (
        <EmptyState message={emptyMessage} />
      ) : (
        children
      )}
    </section>
  );
}

function ProjectCard({
  locale,
  metaLabel,
  metaValue,
  project,
  t,
  tCommon,
}: {
  locale: string;
  metaLabel: string;
  metaValue: string;
  project: ProjectListItem;
  t: any;
  tCommon: any;
}) {
  const statusLabel = t(`overview.status.${project.status}`) || project.status;
  const enabledPublications = getEnabledPublications(project);

  return (
    <Card className="min-h-48">
      <CardHeader className="gap-3">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <CardTitle className="truncate text-base">
              {project.title}
            </CardTitle>
            <CardDescription>
              {t("collab.hub.project.updatedAt", {
                date: formatOptionalDashboardDate(
                  project.updated_at,
                  locale,
                  t("collab.hub.project.none"),
                ),
              })}
            </CardDescription>
          </div>
          <ProjectStatusBadge label={statusLabel} status={project.status} />
        </div>
      </CardHeader>
      <CardContent className="flex flex-1 flex-col justify-between gap-5">
        <div className="space-y-3">
          <div className="flex items-center justify-between gap-3 text-sm">
            <span className="text-muted-foreground">{metaLabel}</span>
            <Badge variant="secondary">{metaValue}</Badge>
          </div>
          <PlatformIconRow
            label={t("overview.table.channel")}
            publications={enabledPublications}
            emptyLabel={t("collab.hub.project.none")}
            tCommon={tCommon}
          />
        </div>
        <Button
          type="button"
          variant="outline"
          className="w-full justify-center"
          nativeButton={false}
          render={(buttonProps) => (
            <Link
              href={`/${locale}/dashboard/content/${project.id}`}
              {...buttonProps}
            >
              <ArrowRight className="size-4" />
              {t("collab.hub.project.open")}
            </Link>
          )}
        />
      </CardContent>
    </Card>
  );
}

function ProjectGrid({ children }: { children: React.ReactNode }) {
  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">{children}</div>
  );
}

function projectRoleLabel(t: any, role: ProjectListItem["role"]) {
  return t(`content.header.role.${role}`);
}

export function CollaborationHubPage() {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const { t: tCommon } = useTranslation(locale, "common");
  const workspaceSelection = useDashboardWorkspaceSelection();

  const [allProjects, setAllProjects] = useState<ProjectListItem[]>([]);
  const [sharedByMeProjects, setSharedByMeProjects] = useState<
    ProjectWithCollaborators[]
  >([]);
  const [sharedProjectsLoading, setSharedProjectsLoading] = useState(true);
  const [sharedProjectsError, setSharedProjectsError] = useState("");
  const [workspaceProjects, setWorkspaceProjects] = useState<ProjectListItem[]>(
    [],
  );
  const [workspaceProjectsLoading, setWorkspaceProjectsLoading] =
    useState(false);
  const [workspaceProjectsError, setWorkspaceProjectsError] = useState("");

  const loadSharedProjects = useCallback(async () => {
    setSharedProjectsLoading(true);
    setSharedProjectsError("");

    try {
      const response = await getDashboardProjects(50);
      const projects = response.items;
      setAllProjects(projects);

      const ownedProjects = getOwnedProjects(projects);
      const collaboratorResults = await Promise.allSettled(
        ownedProjects.map(async (project) => {
          const collaborators = await getProjectCollaborators(project.id);
          return { collaborators: collaborators.items, project };
        }),
      );
      const fulfilledResults = collaboratorResults
        .filter((result) => result.status === "fulfilled")
        .map((result) => result.value);
      setSharedByMeProjects(getProjectsSharedByMe(fulfilledResults));

      if (collaboratorResults.some((result) => result.status === "rejected")) {
        setSharedProjectsError(t("collab.hub.error.defaultMessage"));
      }
    } catch (requestError) {
      setAllProjects([]);
      setSharedByMeProjects([]);
      setSharedProjectsError(
        requestError instanceof Error
          ? requestError.message
          : t("collab.hub.error.defaultMessage"),
      );
    } finally {
      setSharedProjectsLoading(false);
    }
  }, [t]);

  const loadWorkspaceProjects = useCallback(async () => {
    const workspaceId = workspaceSelection.selectedWorkspaceId;
    if (!workspaceId) {
      setWorkspaceProjects([]);
      setWorkspaceProjectsLoading(false);
      return;
    }

    setWorkspaceProjectsLoading(true);
    setWorkspaceProjectsError("");

    try {
      const response = await getWorkspaceProjects(workspaceId, { limit: 50 });
      setWorkspaceProjects(response.items);
    } catch (requestError) {
      setWorkspaceProjects([]);
      setWorkspaceProjectsError(
        requestError instanceof Error
          ? requestError.message
          : t("collab.hub.error.defaultMessage"),
      );
    } finally {
      setWorkspaceProjectsLoading(false);
    }
  }, [t, workspaceSelection.selectedWorkspaceId]);

  useEffect(() => {
    void loadSharedProjects();
  }, [loadSharedProjects]);

  useEffect(() => {
    void loadWorkspaceProjects();
  }, [loadWorkspaceProjects]);

  const sharedWithMeProjects = useMemo(
    () => getProjectsSharedWithMe(allProjects),
    [allProjects],
  );
  const workspaceRoleLabel = workspaceSelection.selectedWorkspace
    ? t(`workspace.role.${workspaceSelection.selectedWorkspace.role}`)
    : t("collab.hub.project.workspaceFallback");
  const isWorkspaceLoading =
    workspaceSelection.isLoading || workspaceProjectsLoading;
  const isRefreshing = sharedProjectsLoading || isWorkspaceLoading;

  const handleRefresh = () => {
    void loadSharedProjects();
    void workspaceSelection.reloadWorkspaces();
    void loadWorkspaceProjects();
  };

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 className="text-3xl font-bold tracking-tight">
            {t("collab.hub.title")}
          </h2>
          <p className="text-muted-foreground">{t("collab.hub.description")}</p>
        </div>
        <div className="flex w-full flex-col gap-2 sm:w-auto sm:flex-row">
          <WorkspaceSwitcher
            disabled={workspaceProjectsLoading}
            isLoading={workspaceSelection.isLoading}
            selectedWorkspace={workspaceSelection.selectedWorkspace}
            workspaces={workspaceSelection.workspaces}
            onWorkspaceChange={workspaceSelection.selectWorkspace}
          />
          <Button
            type="button"
            variant="outline"
            onClick={handleRefresh}
            disabled={isRefreshing}
          >
            <RefreshCw
              className={isRefreshing ? "size-4 animate-spin" : "size-4"}
            />
            {t("collab.hub.refresh")}
          </Button>
        </div>
      </div>

      {sharedProjectsError ? (
        <DashboardErrorCard
          compact
          title={t("collab.hub.error.projectsTitle")}
          message={sharedProjectsError}
          retryLabel={t("workspace.error.retry")}
          onRetry={() => void loadSharedProjects()}
        />
      ) : null}

      {workspaceSelection.error ? (
        <DashboardErrorCard
          compact
          title={t("workspace.error.title")}
          message={workspaceSelection.error}
          retryLabel={t("workspace.error.retry")}
          onRetry={() => void workspaceSelection.reloadWorkspaces()}
        />
      ) : null}

      {workspaceProjectsError ? (
        <DashboardErrorCard
          compact
          title={t("collab.hub.error.workspaceTitle")}
          message={workspaceProjectsError}
          retryLabel={t("workspace.error.retry")}
          onRetry={() => void loadWorkspaceProjects()}
        />
      ) : null}

      <div className="grid gap-4 md:grid-cols-3">
        <DashboardStatCard
          title={t("collab.hub.stats.sharedByMe")}
          value={sharedByMeProjects.length}
          loading={sharedProjectsLoading}
          headerIcon={Share2}
        />
        <DashboardStatCard
          title={t("collab.hub.stats.sharedWithMe")}
          value={sharedWithMeProjects.length}
          loading={sharedProjectsLoading}
          headerIcon={Inbox}
        />
        <DashboardStatCard
          title={t("collab.hub.stats.workspaceProjects")}
          value={workspaceProjects.length}
          loading={isWorkspaceLoading}
          headerIcon={UsersRound}
        />
      </div>

      <CollaborationSection
        title={t("collab.hub.sharedByMe.title")}
        description={t("collab.hub.sharedByMe.description")}
        count={sharedByMeProjects.length}
        isLoading={sharedProjectsLoading}
        emptyMessage={t("collab.hub.sharedByMe.empty")}
      >
        <ProjectGrid>
          {sharedByMeProjects.map(({ collaboratorCount, project }) => (
            <ProjectCard
              key={project.id}
              locale={locale}
              project={project}
              metaLabel={t("collab.hub.sharedByMe.access")}
              metaValue={t("collab.hub.sharedByMe.collaboratorCount", {
                count: collaboratorCount,
              })}
              t={t}
              tCommon={tCommon}
            />
          ))}
        </ProjectGrid>
      </CollaborationSection>

      <CollaborationSection
        title={t("collab.hub.sharedWithMe.title")}
        description={t("collab.hub.sharedWithMe.description")}
        count={sharedWithMeProjects.length}
        isLoading={sharedProjectsLoading}
        emptyMessage={t("collab.hub.sharedWithMe.empty")}
      >
        <ProjectGrid>
          {sharedWithMeProjects.map((project) => (
            <ProjectCard
              key={project.id}
              locale={locale}
              project={project}
              metaLabel={t("collab.hub.sharedWithMe.role")}
              metaValue={projectRoleLabel(t, project.role)}
              t={t}
              tCommon={tCommon}
            />
          ))}
        </ProjectGrid>
      </CollaborationSection>

      <CollaborationSection
        title={t("collab.hub.workspaceProjects.title")}
        description={t("collab.hub.workspaceProjects.description")}
        count={workspaceProjects.length}
        isLoading={isWorkspaceLoading}
        emptyMessage={
          workspaceSelection.selectedWorkspaceId
            ? t("collab.hub.workspaceProjects.empty")
            : t("workspace.empty")
        }
      >
        <ProjectGrid>
          {workspaceProjects.map((project) => (
            <ProjectCard
              key={project.id}
              locale={locale}
              project={project}
              metaLabel={t("collab.hub.workspaceProjects.role")}
              metaValue={workspaceRoleLabel}
              t={t}
              tCommon={tCommon}
            />
          ))}
        </ProjectGrid>
      </CollaborationSection>
    </div>
  );
}

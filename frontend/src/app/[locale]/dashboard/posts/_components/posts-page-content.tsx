"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { CheckCircle2, Pencil, RefreshCw, XCircle } from "lucide-react";
import Link from "next/link";

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
import {
  getEnabledPublications,
  getPublicationTotals,
} from "../../_lib/publications";

function ProjectSkeleton() {
  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      {Array.from({ length: 6 }).map((_, index) => (
        <Skeleton key={index} className="h-56 w-full" />
      ))}
    </div>
  );
}

function PostsStatsGrid({
  loading,
  projects,
  publicationTotals,
  t,
}: {
  loading: boolean;
  projects: ProjectListItem[];
  publicationTotals: ReturnType<typeof getPublicationTotals>;
  t: any;
}) {
  return (
    <div className="grid gap-4 md:grid-cols-3">
      <DashboardStatCard
        title={t("posts.stats.totalProjects")}
        value={projects.length}
        loading={loading}
        skeletonClassName="h-8 w-16"
      />
      <DashboardStatCard
        title={t("posts.stats.publishSuccess")}
        value={
          <>
            <CheckCircle2 className="h-5 w-5 text-primary" />
            {publicationTotals.published}
          </>
        }
        loading={loading}
        skeletonClassName="h-8 w-16"
        valueClassName="flex items-center gap-2 text-2xl font-bold"
      />
      <DashboardStatCard
        title={t("posts.stats.publishFailed")}
        value={
          <>
            <XCircle className="h-5 w-5 text-destructive" />
            {publicationTotals.failed}
          </>
        }
        loading={loading}
        skeletonClassName="h-8 w-16"
        valueClassName="flex items-center gap-2 text-2xl font-bold"
      />
    </div>
  );
}

function PostProjectCard({
  locale,
  project,
  t,
  tCommon,
}: {
  locale: string;
  project: ProjectListItem;
  t: any;
  tCommon: any;
}) {
  const enabledPublications = getEnabledPublications(project);
  const publishedPublications = enabledPublications.filter(
    (publication) => publication.status === "succeeded",
  );
  const failedPublications = enabledPublications.filter(
    (publication) => publication.status === "failed",
  );
  const statusLabel = t(`overview.status.${project.status}`) || project.status;

  return (
    <Card className="flex min-h-56 flex-col">
      <CardHeader className="gap-3">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <CardTitle className="truncate text-lg">{project.title}</CardTitle>
            <CardDescription>
              {t("posts.card.updatedAt", {
                date: formatOptionalDashboardDate(
                  project.updated_at,
                  locale,
                  t("posts.card.none"),
                ),
              })}
            </CardDescription>
          </div>
          <ProjectStatusBadge label={statusLabel} status={project.status} />
        </div>
      </CardHeader>
      <CardContent className="flex flex-1 flex-col justify-between gap-5">
        <div className="space-y-3">
          <PlatformIconRow
            label={t("posts.card.successList")}
            publications={publishedPublications}
            emptyLabel={t("posts.card.none")}
            tCommon={tCommon}
          />
          <PlatformIconRow
            label={t("posts.card.failedList")}
            publications={failedPublications}
            emptyLabel={t("posts.card.none")}
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
              <Pencil className="h-4 w-4" />
              {t("posts.card.edit")}
            </Link>
          )}
        />
      </CardContent>
    </Card>
  );
}

export function PostsPageContent() {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const { t: tCommon } = useTranslation(locale, "common");
  const workspaceSelection = useDashboardWorkspaceSelection();

  const [projects, setProjects] = useState<ProjectListItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const loadPosts = useCallback(async () => {
    const workspaceId = workspaceSelection.selectedWorkspaceId;
    if (!workspaceId) {
      setProjects([]);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError("");

    try {
      const projectsResponse = await getWorkspaceProjects(workspaceId, {
        limit: 20,
      });
      setProjects(projectsResponse.items);
    } catch (requestError) {
      setError(
        requestError instanceof Error
          ? requestError.message
          : t("posts.error.defaultMessage"),
      );
    } finally {
      setLoading(false);
    }
  }, [t, workspaceSelection.selectedWorkspaceId]);

  useEffect(() => {
    void loadPosts();
  }, [loadPosts]);

  const publicationTotals = useMemo(
    () => getPublicationTotals(projects),
    [projects],
  );
  const isPageLoading = workspaceSelection.isLoading || loading;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 className="text-3xl font-bold tracking-tight">
            {t("posts.title")}
          </h2>
          <p className="text-muted-foreground">{t("posts.description")}</p>
        </div>
        <div className="flex w-full flex-col gap-2 sm:w-auto sm:flex-row">
          <WorkspaceSwitcher
            disabled={loading}
            isLoading={workspaceSelection.isLoading}
            selectedWorkspace={workspaceSelection.selectedWorkspace}
            workspaces={workspaceSelection.workspaces}
            onWorkspaceChange={workspaceSelection.selectWorkspace}
            onWorkspaceCreate={workspaceSelection.createWorkspace}
            isCreatingWorkspace={workspaceSelection.isCreatingWorkspace}
          />
          <Button
            type="button"
            variant="outline"
            onClick={() => void loadPosts()}
            disabled={isPageLoading || !workspaceSelection.selectedWorkspaceId}
          >
            <RefreshCw className="h-4 w-4" />
            {t("posts.refresh")}
          </Button>
        </div>
      </div>

      {workspaceSelection.error ? (
        <DashboardErrorCard
          compact
          title={t("workspace.error.title")}
          message={workspaceSelection.error}
          retryLabel={t("workspace.error.retry")}
          onRetry={() => void workspaceSelection.reloadWorkspaces()}
        />
      ) : null}

      {error ? (
        <DashboardErrorCard
          compact
          title={t("posts.error.title")}
          message={error}
        />
      ) : null}

      <PostsStatsGrid
        loading={isPageLoading}
        projects={projects}
        publicationTotals={publicationTotals}
        t={t}
      />

      {isPageLoading ? (
        <ProjectSkeleton />
      ) : !workspaceSelection.selectedWorkspaceId ? (
        <div className="flex min-h-56 items-center justify-center rounded-lg border border-dashed text-sm text-muted-foreground">
          {t("workspace.empty")}
        </div>
      ) : projects.length === 0 ? (
        <div className="flex min-h-56 items-center justify-center rounded-lg border border-dashed text-sm text-muted-foreground">
          {t("posts.empty")}
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {projects.map((project) => (
            <PostProjectCard
              key={project.id}
              locale={locale}
              project={project}
              t={t}
              tCommon={tCommon}
            />
          ))}
        </div>
      )}
    </div>
  );
}

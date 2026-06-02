"use client";

import { useEffect, useMemo, useState } from "react";
import {
  CheckCircle2,
  ExternalLink,
  FileText,
  RefreshCw,
  Share2,
  XCircle,
} from "lucide-react";
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
  getDashboardProjects,
  getDashboardStats,
  type DashboardStats,
  type ProjectListItem,
} from "@/lib/dashboard/api";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";

import { DashboardErrorCard } from "./dashboard-error-card";
import { DashboardStatCard } from "./dashboard-stat-card";
import { ProjectStatusBadge } from "./project-status-badge";
import { PublicationBadgeList } from "./publication-platforms";
import { formatDashboardDate, formatDashboardNumber } from "../_lib/formatters";
import {
  getEnabledPlatformCount,
  getEnabledPublications,
} from "../_lib/publications";

function ProjectListSkeleton() {
  return (
    <div className="space-y-3">
      {Array.from({ length: 5 }).map((_, index) => (
        <Skeleton key={index} className="h-12 w-full" />
      ))}
    </div>
  );
}

function RecentProjectsCard({
  loading,
  locale,
  onRefresh,
  projects,
  t,
  tCommon,
  totalProjects,
}: {
  loading: boolean;
  locale: string;
  onRefresh: () => void;
  projects: ProjectListItem[];
  t: any;
  tCommon: any;
  totalProjects: number;
}) {
  return (
    <Card>
      <CardHeader className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <CardTitle>{t("overview.recent.title")}</CardTitle>
          <CardDescription>
            {t("overview.recent.description", {
              total: formatDashboardNumber(totalProjects, locale),
            })}
          </CardDescription>
        </div>
        <div className="flex gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            nativeButton={false}
            render={(buttonProps) => (
              <Link href={`/${locale}/dashboard/posts`} {...buttonProps}>
                <ExternalLink className="h-4 w-4" />
                {t("overview.recent.viewPosts")}
              </Link>
            )}
          />
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={onRefresh}
            disabled={loading}
          >
            <RefreshCw className="h-4 w-4" />
            {t("overview.recent.refresh")}
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {loading ? (
          <ProjectListSkeleton />
        ) : projects.length === 0 ? (
          <div className="flex min-h-48 items-center justify-center rounded-lg border border-dashed text-sm text-muted-foreground">
            {t("overview.recent.empty")}
          </div>
        ) : (
          <div className="overflow-hidden rounded-md border">
            <table className="w-full text-sm">
              <thead className="bg-muted/50 text-left text-muted-foreground">
                <tr>
                  <th className="px-4 py-3 font-medium">
                    {t("overview.table.title")}
                  </th>
                  <th className="hidden px-4 py-3 font-medium md:table-cell">
                    {t("overview.table.status")}
                  </th>
                  <th className="hidden px-4 py-3 font-medium lg:table-cell">
                    {t("overview.table.channel")}
                  </th>
                  <th className="px-4 py-3 text-right font-medium">
                    {t("overview.table.updated")}
                  </th>
                </tr>
              </thead>
              <tbody>
                {projects.map((project) => {
                  const statusLabel =
                    t(`overview.status.${project.status}`) || project.status;

                  return (
                    <tr key={project.id} className="border-t">
                      <td className="max-w-0 px-4 py-3">
                        <div className="truncate font-medium">
                          {project.title}
                        </div>
                        <div className="mt-1 md:hidden">
                          <ProjectStatusBadge
                            label={statusLabel}
                            status={project.status}
                          />
                        </div>
                      </td>
                      <td className="hidden px-4 py-3 md:table-cell">
                        <ProjectStatusBadge
                          label={statusLabel}
                          status={project.status}
                        />
                      </td>
                      <td className="hidden px-4 py-3 lg:table-cell">
                        <PublicationBadgeList
                          emptyLabel={t("overview.status.unconfigured")}
                          publications={getEnabledPublications(project)}
                          tCommon={tCommon}
                        />
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 text-right text-muted-foreground">
                        {formatDashboardDate(project.updated_at, locale)}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

export function DashboardOverviewPage() {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const { t: tCommon } = useTranslation(locale, "common");

  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [projects, setProjects] = useState<ProjectListItem[]>([]);
  const [totalProjects, setTotalProjects] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const loadDashboard = async () => {
    setLoading(true);
    setError("");

    try {
      const [statsResponse, projectsResponse] = await Promise.all([
        getDashboardStats(),
        getDashboardProjects(),
      ]);

      setStats(statsResponse);
      setProjects(projectsResponse.items);
      setTotalProjects(projectsResponse.total);
    } catch (requestError) {
      setError(
        requestError instanceof Error
          ? requestError.message
          : t("overview.error.defaultMessage"),
      );
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadDashboard();
  }, []);

  const enabledChannelCount = useMemo(
    () => getEnabledPlatformCount(projects),
    [projects],
  );

  const publishedCount = stats?.total_published_publications ?? 0;
  const failedCount = stats?.total_failed_publications ?? 0;
  const successRate =
    publishedCount + failedCount === 0
      ? 0
      : Math.round((publishedCount / (publishedCount + failedCount)) * 100);

  return (
    <div className="flex flex-col gap-4">
      {error ? (
        <DashboardErrorCard
          title={t("overview.error.title")}
          message={error}
          retryLabel={t("overview.error.retry")}
          onRetry={() => void loadDashboard()}
        />
      ) : null}

      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <DashboardStatCard
          title={t("overview.stats.totalProjects")}
          value={formatDashboardNumber(stats?.total_projects ?? 0, locale)}
          description={t("overview.stats.totalProjectsDesc")}
          headerIcon={FileText}
          loading={loading}
        />
        <DashboardStatCard
          title={t("overview.stats.activeChannels")}
          value={formatDashboardNumber(enabledChannelCount, locale)}
          description={t("overview.stats.activeChannelsDesc")}
          headerIcon={Share2}
          loading={loading}
        />
        <DashboardStatCard
          title={t("overview.stats.publishSuccess")}
          value={formatDashboardNumber(publishedCount, locale)}
          description={t("overview.stats.publishSuccessDesc", {
            rate: successRate,
          })}
          headerIcon={CheckCircle2}
          loading={loading}
        />
        <DashboardStatCard
          title={t("overview.stats.publishFailed")}
          value={formatDashboardNumber(failedCount, locale)}
          description={t("overview.stats.publishFailedDesc")}
          headerIcon={XCircle}
          loading={loading}
        />
      </div>

      <RecentProjectsCard
        loading={loading}
        locale={locale}
        onRefresh={() => void loadDashboard()}
        projects={projects}
        t={t}
        tCommon={tCommon}
        totalProjects={totalProjects}
      />
    </div>
  );
}

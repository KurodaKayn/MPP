"use client";

import { useEffect, useMemo, useState } from "react";
import {
  AlertCircle,
  CheckCircle2,
  ExternalLink,
  FileText,
  RefreshCw,
  Share2,
  XCircle,
} from "lucide-react";
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
import { PLATFORM_TABS } from "@/lib/content/platforms";
import {
  getDashboardProjects,
  getDashboardStats,
  type DashboardStats,
  type ProjectListItem,
} from "@/lib/dashboard/api";

const statusLabels: Record<string, string> = {
  draft: "草稿",
  ready: "就绪",
  publishing: "发布中",
  published: "已发布",
  failed: "失败",
};

const statusVariants: Record<
  string,
  React.ComponentProps<typeof Badge>["variant"]
> = {
  draft: "outline",
  ready: "secondary",
  publishing: "secondary",
  published: "default",
  failed: "destructive",
};

function formatNumber(value: number) {
  return new Intl.NumberFormat("zh-CN").format(value);
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function getPlatformLabel(platform: string) {
  return (
    PLATFORM_TABS.find((item) => item.value === platform)?.label ?? platform
  );
}

function MetricCard({
  title,
  value,
  description,
  icon: Icon,
  loading,
}: {
  title: string;
  value: number;
  description: string;
  icon: React.ComponentType<{ className?: string }>;
  loading: boolean;
}) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
        <Icon className="h-4 w-4 text-muted-foreground" />
      </CardHeader>
      <CardContent>
        {loading ? (
          <Skeleton className="h-8 w-24" />
        ) : (
          <div className="text-2xl font-bold">{formatNumber(value)}</div>
        )}
        <p className="mt-1 text-xs text-muted-foreground">{description}</p>
      </CardContent>
    </Card>
  );
}

function ProjectStatus({ status }: { status: string }) {
  return (
    <Badge variant={statusVariants[status] ?? "outline"}>
      {statusLabels[status] ?? status}
    </Badge>
  );
}

function PublicationStatus({ project }: { project: ProjectListItem }) {
  const enabledPublications = project.publications.filter(
    (publication) => publication.enabled,
  );

  if (enabledPublications.length === 0) {
    return <span className="text-muted-foreground">未配置</span>;
  }

  return (
    <div className="flex flex-wrap gap-1.5">
      {enabledPublications.map((publication) => (
        <Badge
          key={publication.id}
          variant={publication.status === "failed" ? "destructive" : "outline"}
        >
          {getPlatformLabel(publication.platform)}
        </Badge>
      ))}
    </div>
  );
}

function ProjectListSkeleton() {
  return (
    <div className="space-y-3">
      {Array.from({ length: 5 }).map((_, index) => (
        <Skeleton key={index} className="h-12 w-full" />
      ))}
    </div>
  );
}

export default function DashboardPage() {
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
          : "无法加载控制台数据",
      );
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadDashboard();
  }, []);

  const enabledChannelCount = useMemo(() => {
    const platforms = new Set<string>();

    for (const project of projects) {
      for (const publication of project.publications) {
        if (publication.enabled) {
          platforms.add(publication.platform);
        }
      }
    }

    return platforms.size;
  }, [projects]);

  const publishedCount = stats?.total_published_publications ?? 0;
  const failedCount = stats?.total_failed_publications ?? 0;
  const successRate =
    publishedCount + failedCount === 0
      ? 0
      : Math.round((publishedCount / (publishedCount + failedCount)) * 100);

  return (
    <div className="flex flex-col gap-4">
      {error ? (
        <Card className="border-destructive/40 bg-destructive/5">
          <CardContent className="flex flex-col gap-3 py-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex items-start gap-3">
              <AlertCircle className="mt-0.5 h-4 w-4 text-destructive" />
              <div>
                <div className="text-sm font-medium">控制台数据加载失败</div>
                <p className="text-sm text-muted-foreground">{error}</p>
              </div>
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => void loadDashboard()}
            >
              <RefreshCw className="h-4 w-4" />
              重试
            </Button>
          </CardContent>
        </Card>
      ) : null}

      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <MetricCard
          title="内容总数"
          value={stats?.total_projects ?? 0}
          description="当前系统内的项目数量"
          icon={FileText}
          loading={loading}
        />
        <MetricCard
          title="活跃渠道"
          value={enabledChannelCount}
          description="最近项目覆盖的平台数"
          icon={Share2}
          loading={loading}
        />
        <MetricCard
          title="发布成功"
          value={publishedCount}
          description={`成功率 ${successRate}%`}
          icon={CheckCircle2}
          loading={loading}
        />
        <MetricCard
          title="发布失败"
          value={failedCount}
          description="需要排查的分发记录"
          icon={XCircle}
          loading={loading}
        />
      </div>

      <Card>
        <CardHeader className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <CardTitle>最近内容</CardTitle>
            <CardDescription>
              共 {formatNumber(totalProjects)} 篇内容，显示最近 8 篇。
            </CardDescription>
          </div>
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              render={(buttonProps) => (
                <Link href="/dashboard/posts" {...buttonProps}>
                  <ExternalLink className="h-4 w-4" />
                  查看发布
                </Link>
              )}
            />
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => void loadDashboard()}
              disabled={loading}
            >
              <RefreshCw className="h-4 w-4" />
              刷新
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {loading ? (
            <ProjectListSkeleton />
          ) : projects.length === 0 ? (
            <div className="flex min-h-48 items-center justify-center rounded-lg border border-dashed text-sm text-muted-foreground">
              暂无内容数据
            </div>
          ) : (
            <div className="overflow-hidden rounded-md border">
              <table className="w-full text-sm">
                <thead className="bg-muted/50 text-left text-muted-foreground">
                  <tr>
                    <th className="px-4 py-3 font-medium">标题</th>
                    <th className="hidden px-4 py-3 font-medium md:table-cell">
                      状态
                    </th>
                    <th className="hidden px-4 py-3 font-medium lg:table-cell">
                      渠道
                    </th>
                    <th className="px-4 py-3 text-right font-medium">更新</th>
                  </tr>
                </thead>
                <tbody>
                  {projects.map((project) => (
                    <tr key={project.id} className="border-t">
                      <td className="max-w-0 px-4 py-3">
                        <div className="truncate font-medium">
                          {project.title}
                        </div>
                        <div className="mt-1 md:hidden">
                          <ProjectStatus status={project.status} />
                        </div>
                      </td>
                      <td className="hidden px-4 py-3 md:table-cell">
                        <ProjectStatus status={project.status} />
                      </td>
                      <td className="hidden px-4 py-3 lg:table-cell">
                        <PublicationStatus project={project} />
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 text-right text-muted-foreground">
                        {formatDate(project.updated_at)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

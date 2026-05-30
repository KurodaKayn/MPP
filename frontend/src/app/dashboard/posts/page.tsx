"use client";

import { useEffect, useMemo, useState } from "react";
import {
  AlertCircle,
  CheckCircle2,
  ExternalLink,
  Loader2,
  RefreshCw,
  Send,
  XCircle,
} from "lucide-react";
import Image from "next/image";
import { toast } from "sonner";
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
  getProjectPublications,
  publishProject,
  type ProjectListItem,
  type PublicationDetail,
} from "@/lib/dashboard/api";

const statusLabels: Record<string, string> = {
  adapted: "已适配",
  disabled: "已停用",
  draft: "草稿",
  failed: "失败",
  pending: "待处理",
  published: "已发布",
  publishing: "发布中",
  ready: "就绪",
};

const statusVariants: Record<
  string,
  React.ComponentProps<typeof Badge>["variant"]
> = {
  adapted: "secondary",
  disabled: "outline",
  failed: "destructive",
  pending: "outline",
  published: "default",
  publishing: "secondary",
  ready: "secondary",
};

function getPlatform(platform: string) {
  return PLATFORM_TABS.find((item) => item.value === platform);
}

function formatDate(value?: string) {
  if (!value) {
    return "暂无";
  }

  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function StatusBadge({ status }: { status: string }) {
  return (
    <Badge variant={statusVariants[status] ?? "outline"}>
      {statusLabels[status] ?? status}
    </Badge>
  );
}

function PlatformName({ platform }: { platform: string }) {
  const metadata = getPlatform(platform);

  return (
    <span className="inline-flex min-w-0 items-center gap-2">
      {metadata ? (
        <Image
          src={metadata.icon}
          alt=""
          width={18}
          height={18}
          aria-hidden="true"
          className="size-[18px] shrink-0"
        />
      ) : null}
      <span className="truncate">{metadata?.label ?? platform}</span>
    </span>
  );
}

function getContentSummary(publication: PublicationDetail) {
  const summary = publication.adapted_content?.summary;

  return typeof summary === "string" && summary.trim()
    ? summary
    : "暂无适配摘要";
}

function ProjectSkeleton() {
  return (
    <div className="space-y-3">
      {Array.from({ length: 3 }).map((_, index) => (
        <Skeleton key={index} className="h-36 w-full" />
      ))}
    </div>
  );
}

export default function PostsPage() {
  const [projects, setProjects] = useState<ProjectListItem[]>([]);
  const [publicationsByProject, setPublicationsByProject] = useState<
    Record<string, PublicationDetail[]>
  >({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [publishingKey, setPublishingKey] = useState("");

  const loadPosts = async () => {
    setLoading(true);
    setError("");

    try {
      const projectsResponse = await getDashboardProjects(20);
      const publicationResponses = await Promise.all(
        projectsResponse.items.map((project) =>
          getProjectPublications(project.id),
        ),
      );
      const nextPublications = Object.fromEntries(
        publicationResponses.map((response) => [
          response.project_id,
          response.items,
        ]),
      );

      setProjects(projectsResponse.items);
      setPublicationsByProject(nextPublications);
    } catch (requestError) {
      setError(
        requestError instanceof Error
          ? requestError.message
          : "无法加载内容列表",
      );
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadPosts();
  }, []);

  const publicationTotals = useMemo(() => {
    const items = Object.values(publicationsByProject).flat();

    return {
      failed: items.filter((item) => item.status === "failed").length,
      published: items.filter((item) => item.status === "published").length,
      total: items.length,
    };
  }, [publicationsByProject]);

  const handlePublish = async (projectId: string, platform: string) => {
    const key = `${projectId}:${platform}`;
    setPublishingKey(key);

    try {
      const result = await publishProject(projectId, platform);
      const refreshed = await getProjectPublications(projectId);

      setPublicationsByProject((current) => ({
        ...current,
        [projectId]: refreshed.items,
      }));

      if (result.status === "failed") {
        toast.error("发布失败", {
          description: result.error_message || "平台返回失败状态。",
        });
      } else {
        toast.success("发布完成", {
          description: "平台记录已更新。",
        });
      }
    } catch (requestError) {
      toast.error("发布请求失败", {
        description:
          requestError instanceof Error ? requestError.message : "请稍后重试。",
      });
    } finally {
      setPublishingKey("");
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="text-muted-foreground">
            查看各平台适配状态，并触发后端发布流程。
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          onClick={() => void loadPosts()}
          disabled={loading}
        >
          <RefreshCw className="h-4 w-4" />
          刷新
        </Button>
      </div>

      {error ? (
        <Card className="border-destructive/40 bg-destructive/5">
          <CardContent className="flex items-start gap-3 py-4">
            <AlertCircle className="mt-0.5 h-4 w-4 text-destructive" />
            <div>
              <div className="text-sm font-medium">内容列表加载失败</div>
              <p className="text-sm text-muted-foreground">{error}</p>
            </div>
          </CardContent>
        </Card>
      ) : null}

      <div className="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">项目数量</CardTitle>
          </CardHeader>
          <CardContent className="text-2xl font-bold">
            {loading ? <Skeleton className="h-8 w-16" /> : projects.length}
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">已发布渠道</CardTitle>
          </CardHeader>
          <CardContent className="flex items-center gap-2 text-2xl font-bold">
            {loading ? (
              <Skeleton className="h-8 w-16" />
            ) : (
              <>
                <CheckCircle2 className="h-5 w-5 text-primary" />
                {publicationTotals.published}
              </>
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">失败渠道</CardTitle>
          </CardHeader>
          <CardContent className="flex items-center gap-2 text-2xl font-bold">
            {loading ? (
              <Skeleton className="h-8 w-16" />
            ) : (
              <>
                <XCircle className="h-5 w-5 text-destructive" />
                {publicationTotals.failed}
              </>
            )}
          </CardContent>
        </Card>
      </div>

      {loading ? (
        <ProjectSkeleton />
      ) : projects.length === 0 ? (
        <div className="flex min-h-56 items-center justify-center rounded-lg border border-dashed text-sm text-muted-foreground">
          暂无内容数据
        </div>
      ) : (
        <div className="space-y-4">
          {projects.map((project) => {
            const publications = publicationsByProject[project.id] ?? [];

            return (
              <Card key={project.id}>
                <CardHeader className="gap-2">
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div className="min-w-0">
                      <CardTitle className="truncate text-lg">
                        {project.title}
                      </CardTitle>
                      <CardDescription>
                        更新于 {formatDate(project.updated_at)}
                      </CardDescription>
                    </div>
                    <StatusBadge status={project.status} />
                  </div>
                </CardHeader>
                <CardContent>
                  <div className="overflow-hidden rounded-md border">
                    <table className="w-full text-sm">
                      <thead className="bg-muted/50 text-left text-muted-foreground">
                        <tr>
                          <th className="px-4 py-3 font-medium">渠道</th>
                          <th className="hidden px-4 py-3 font-medium md:table-cell">
                            适配摘要
                          </th>
                          <th className="px-4 py-3 font-medium">状态</th>
                          <th className="hidden px-4 py-3 font-medium lg:table-cell">
                            最近发布
                          </th>
                          <th className="px-4 py-3 text-right font-medium">
                            操作
                          </th>
                        </tr>
                      </thead>
                      <tbody>
                        {publications.map((publication) => {
                          const key = `${project.id}:${publication.platform}`;
                          const disabled =
                            !publication.enabled || publishingKey === key;

                          return (
                            <tr key={publication.id} className="border-t">
                              <td className="max-w-32 px-4 py-3 font-medium">
                                <PlatformName platform={publication.platform} />
                              </td>
                              <td className="hidden max-w-0 px-4 py-3 md:table-cell">
                                <div className="truncate text-muted-foreground">
                                  {getContentSummary(publication)}
                                </div>
                                {publication.error_message ? (
                                  <div className="mt-1 truncate text-xs text-destructive">
                                    {publication.error_message}
                                  </div>
                                ) : null}
                              </td>
                              <td className="px-4 py-3">
                                <StatusBadge status={publication.status} />
                              </td>
                              <td className="hidden whitespace-nowrap px-4 py-3 text-muted-foreground lg:table-cell">
                                {formatDate(publication.published_at)}
                              </td>
                              <td className="px-4 py-3">
                                <div className="flex justify-end gap-2">
                                  {publication.publish_url ? (
                                    <Button
                                      type="button"
                                      size="icon"
                                      variant="outline"
                                      title="打开发布链接"
                                      nativeButton={false}
                                      render={(buttonProps) => (
                                        <a
                                          href={publication.publish_url}
                                          target="_blank"
                                          rel="noreferrer"
                                          {...buttonProps}
                                        >
                                          <ExternalLink className="h-4 w-4" />
                                        </a>
                                      )}
                                    />
                                  ) : null}
                                  <Button
                                    type="button"
                                    size="sm"
                                    onClick={() =>
                                      void handlePublish(
                                        project.id,
                                        publication.platform,
                                      )
                                    }
                                    disabled={disabled}
                                  >
                                    {publishingKey === key ? (
                                      <Loader2 className="h-4 w-4 animate-spin" />
                                    ) : (
                                      <Send className="h-4 w-4" />
                                    )}
                                    发布
                                  </Button>
                                </div>
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}

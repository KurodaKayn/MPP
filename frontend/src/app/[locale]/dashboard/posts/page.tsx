"use client";

import { useEffect, useMemo, useState } from "react";
import {
  AlertCircle,
  CheckCircle2,
  Pencil,
  RefreshCw,
  XCircle,
} from "lucide-react";
import Image from "next/image";
import Link from "next/link";
import { useParams } from "next/navigation";
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
  type ProjectListItem,
} from "@/lib/dashboard/api";
import { useTranslation } from "@/lib/i18n/client";
import { getIntlLocale } from "@/lib/i18n/settings";

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

type PublicationSummary = ProjectListItem["publications"][number];

function getPlatform(platform: string) {
  return PLATFORM_TABS.find((item) => item.value === platform);
}

function formatDate(value: string | undefined, locale: string, t: any) {
  if (!value) {
    return t("posts.card.none");
  }

  return new Intl.DateTimeFormat(getIntlLocale(locale), {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function StatusBadge({ status, t }: { status: string; t: any }) {
  const statusLabel = t(`overview.status.${status}`) || status;
  return (
    <Badge variant={statusVariants[status] ?? "outline"}>{statusLabel}</Badge>
  );
}

function PlatformIcon({
  platform,
  tCommon,
}: {
  platform: string;
  tCommon: any;
}) {
  const metadata = getPlatform(platform);

  if (!metadata) {
    return (
      <span
        aria-label={platform}
        className="flex size-7 items-center justify-center rounded-md border bg-muted text-[10px] font-semibold uppercase text-muted-foreground"
      >
        {platform.slice(0, 2)}
      </span>
    );
  }

  return (
    <span
      className="flex size-7 items-center justify-center rounded-md border bg-background"
      title={tCommon(metadata.label)}
    >
      <Image
        src={metadata.icon}
        alt={tCommon(metadata.label)}
        width={18}
        height={18}
        className="size-[18px]"
      />
    </span>
  );
}

function PlatformIconRow({
  label,
  publications,
  t,
  tCommon,
}: {
  label: string;
  publications: PublicationSummary[];
  t: any;
  tCommon: any;
}) {
  return (
    <div className="grid grid-cols-[4.75rem_minmax(0,1fr)] items-center gap-3 text-sm">
      <div className="whitespace-nowrap text-muted-foreground">{label}:</div>
      <div className="flex min-h-7 flex-wrap items-center gap-2">
        {publications.length > 0 ? (
          publications.map((publication) => (
            <PlatformIcon
              key={`${publication.id}-${publication.platform}`}
              platform={publication.platform}
              tCommon={tCommon}
            />
          ))
        ) : (
          <span className="text-xs text-muted-foreground">
            {t("posts.card.none")}
          </span>
        )}
      </div>
    </div>
  );
}

function ProjectSkeleton() {
  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      {Array.from({ length: 6 }).map((_, index) => (
        <Skeleton key={index} className="h-56 w-full" />
      ))}
    </div>
  );
}

export default function PostsPage() {
  const params = useParams();
  const locale = (params?.locale as string) || "en";
  const { t } = useTranslation(locale, "dashboard");
  const { t: tCommon } = useTranslation(locale, "common");

  const [projects, setProjects] = useState<ProjectListItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const loadPosts = async () => {
    setLoading(true);
    setError("");

    try {
      const projectsResponse = await getDashboardProjects(20);
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
  };

  useEffect(() => {
    void loadPosts();
  }, []);

  const publicationTotals = useMemo(() => {
    const items = projects.flatMap((project) => project.publications);

    return {
      failed: items.filter((item) => item.enabled && item.status === "failed")
        .length,
      published: items.filter(
        (item) => item.enabled && item.status === "published",
      ).length,
      total: items.filter((item) => item.enabled).length,
    };
  }, [projects]);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 className="text-3xl font-bold tracking-tight">
            {t("posts.title")}
          </h2>
          <p className="text-muted-foreground">{t("posts.description")}</p>
        </div>
        <Button
          type="button"
          variant="outline"
          onClick={() => void loadPosts()}
          disabled={loading}
        >
          <RefreshCw className="h-4 w-4" />
          {t("posts.refresh")}
        </Button>
      </div>

      {error ? (
        <Card className="border-destructive/40 bg-destructive/5">
          <CardContent className="flex items-start gap-3 py-4">
            <AlertCircle className="mt-0.5 h-4 w-4 text-destructive" />
            <div>
              <div className="text-sm font-medium">
                {t("posts.error.title")}
              </div>
              <p className="text-sm text-muted-foreground">{error}</p>
            </div>
          </CardContent>
        </Card>
      ) : null}

      <div className="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">
              {t("posts.stats.totalProjects")}
            </CardTitle>
          </CardHeader>
          <CardContent className="text-2xl font-bold">
            {loading ? <Skeleton className="h-8 w-16" /> : projects.length}
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">
              {t("posts.stats.publishSuccess")}
            </CardTitle>
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
            <CardTitle className="text-sm font-medium">
              {t("posts.stats.publishFailed")}
            </CardTitle>
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
          {t("posts.empty")}
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {projects.map((project) => {
            const enabledPublications = project.publications.filter(
              (publication) => publication.enabled,
            );
            const publishedPublications = enabledPublications.filter(
              (publication) => publication.status === "published",
            );
            const failedPublications = enabledPublications.filter(
              (publication) => publication.status === "failed",
            );

            return (
              <Card key={project.id} className="flex min-h-56 flex-col">
                <CardHeader className="gap-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <CardTitle className="truncate text-lg">
                        {project.title}
                      </CardTitle>
                      <CardDescription>
                        {t("posts.card.updatedAt", {
                          date: formatDate(project.updated_at, locale, t),
                        })}
                      </CardDescription>
                    </div>
                    <StatusBadge status={project.status} t={t} />
                  </div>
                </CardHeader>
                <CardContent className="flex flex-1 flex-col justify-between gap-5">
                  <div className="space-y-3">
                    <PlatformIconRow
                      label={t("posts.card.successList")}
                      publications={publishedPublications}
                      t={t}
                      tCommon={tCommon}
                    />
                    <PlatformIconRow
                      label={t("posts.card.failedList")}
                      publications={failedPublications}
                      t={t}
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
          })}
        </div>
      )}
    </div>
  );
}

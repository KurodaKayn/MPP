"use client";

import {
  Activity,
  Check,
  History,
  Link2,
  Loader2,
  MessageSquareText,
  RefreshCw,
  RotateCcw,
} from "lucide-react";
import type { ReactNode } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import { formatDashboardDate } from "../../_lib/formatters";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  createProjectComment,
  createProjectShareLink,
  getProjectActivities,
  getProjectComments,
  getProjectShareLinks,
  getProjectVersions,
  restoreProjectVersion,
  revokeProjectShareLink,
  updateProjectComment,
  type ProjectActivity,
  type ProjectComment,
  type ProjectRole,
  type ProjectShareLink,
  type ProjectVersion,
} from "@/lib/dashboard/api";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";

type ProjectCollaborationPanelProps = {
  canEdit: boolean;
  onVersionRestore: (project: {
    title: string;
    source_content: string;
  }) => void;
  projectId: string;
  projectRole: ProjectRole | null;
};

export function ProjectCollaborationPanel({
  canEdit,
  onVersionRestore,
  projectId,
  projectRole,
}: ProjectCollaborationPanelProps) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const [activities, setActivities] = useState<ProjectActivity[]>([]);
  const [comments, setComments] = useState<ProjectComment[]>([]);
  const [versions, setVersions] = useState<ProjectVersion[]>([]);
  const [shareLinks, setShareLinks] = useState<ProjectShareLink[]>([]);
  const [commentBody, setCommentBody] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [isSubmittingComment, setIsSubmittingComment] = useState(false);
  const [isCreatingShareLink, setIsCreatingShareLink] = useState(false);
  const canManageShareLinks = projectRole === "owner";

  const loadAll = useCallback(async () => {
    setIsLoading(true);
    try {
      const [activityResp, commentResp, versionResp, linkResp] =
        await Promise.all([
          getProjectActivities(projectId),
          getProjectComments(projectId),
          getProjectVersions(projectId),
          canManageShareLinks
            ? getProjectShareLinks(projectId)
            : Promise.resolve({ items: [] }),
        ]);
      setActivities(activityResp.items);
      setComments(commentResp.items);
      setVersions(versionResp.items);
      setShareLinks(linkResp.items);
    } catch (error) {
      toast.error(t("content.collaboration.loadFailed"), {
        description:
          error instanceof Error
            ? error.message
            : t("content.share.retryLater"),
      });
    } finally {
      setIsLoading(false);
    }
  }, [canManageShareLinks, projectId, t]);

  useEffect(() => {
    void loadAll();
  }, [loadAll]);

  const openComments = useMemo(
    () => comments.filter((comment) => comment.status === "open"),
    [comments],
  );

  const submitComment = async () => {
    const body = commentBody.trim();
    if (!body) {
      return;
    }
    setIsSubmittingComment(true);
    try {
      const comment = await createProjectComment(projectId, { body });
      setComments((items) => [comment, ...items]);
      setCommentBody("");
      void loadAll();
    } catch (error) {
      toast.error(t("content.collaboration.commentFailed"), {
        description:
          error instanceof Error
            ? error.message
            : t("content.share.retryLater"),
      });
    } finally {
      setIsSubmittingComment(false);
    }
  };

  const resolveComment = async (commentId: string) => {
    try {
      const updated = await updateProjectComment(projectId, commentId, {
        status: "resolved",
      });
      setComments((items) =>
        items.map((item) => (item.id === updated.id ? updated : item)),
      );
      void loadAll();
    } catch (error) {
      toast.error(t("content.collaboration.resolveFailed"), {
        description:
          error instanceof Error
            ? error.message
            : t("content.share.retryLater"),
      });
    }
  };

  const restoreVersion = async (version: ProjectVersion) => {
    try {
      const restored = await restoreProjectVersion(projectId, version.id);
      onVersionRestore(restored.project);
      toast.success(t("content.collaboration.versionRestored"));
      void loadAll();
    } catch (error) {
      toast.error(t("content.collaboration.restoreFailed"), {
        description:
          error instanceof Error
            ? error.message
            : t("content.share.retryLater"),
      });
    }
  };

  const createShareLink = async () => {
    setIsCreatingShareLink(true);
    try {
      const link = await createProjectShareLink(projectId, { role: "viewer" });
      setShareLinks((items) => [link, ...items]);
      void loadAll();

      try {
        if (!navigator.clipboard) {
          throw new Error(t("content.collaboration.shareLinkCopyUnavailable"));
        }
        await navigator.clipboard.writeText(link.url);
        toast.success(t("content.collaboration.shareLinkCreated"));
      } catch (copyError) {
        toast.error(t("content.collaboration.shareLinkCopyFailed"), {
          description:
            copyError instanceof Error
              ? copyError.message
              : t("content.share.retryLater"),
        });
      }
    } catch (error) {
      toast.error(t("content.collaboration.shareLinkFailed"), {
        description:
          error instanceof Error
            ? error.message
            : t("content.share.retryLater"),
      });
    } finally {
      setIsCreatingShareLink(false);
    }
  };

  const revokeShareLink = async (linkId: string) => {
    try {
      await revokeProjectShareLink(projectId, linkId);
      setShareLinks((items) =>
        items.map((item) =>
          item.id === linkId ? { ...item, status: "revoked" } : item,
        ),
      );
      void loadAll();
    } catch (error) {
      toast.error(t("content.collaboration.revokeShareLinkFailed"), {
        description:
          error instanceof Error
            ? error.message
            : t("content.share.retryLater"),
      });
    }
  };

  return (
    <Card>
      <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <CardTitle className="text-base">
            {t("content.collaboration.title")}
          </CardTitle>
          <p className="text-sm text-muted-foreground">
            {t("content.collaboration.description")}
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => void loadAll()}
          disabled={isLoading}
        >
          {isLoading ? (
            <Loader2 className="mr-2 size-4 animate-spin" />
          ) : (
            <RefreshCw className="mr-2 size-4" />
          )}
          {t("content.collaboration.refresh")}
        </Button>
      </CardHeader>
      <CardContent>
        <Tabs defaultValue="comments">
          <TabsList className="flex w-full flex-wrap justify-start">
            <TabsTrigger value="comments">
              <MessageSquareText className="size-4" />
              {t("content.collaboration.comments")}
              {openComments.length > 0 ? (
                <Badge variant="secondary">{openComments.length}</Badge>
              ) : null}
            </TabsTrigger>
            <TabsTrigger value="activity">
              <Activity className="size-4" />
              {t("content.collaboration.activity")}
            </TabsTrigger>
            <TabsTrigger value="versions">
              <History className="size-4" />
              {t("content.collaboration.versions")}
            </TabsTrigger>
            <TabsTrigger value="links" disabled={!canManageShareLinks}>
              <Link2 className="size-4" />
              {t("content.collaboration.shareLinks")}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="comments" className="mt-4 space-y-4">
            <div className="space-y-2">
              <Textarea
                value={commentBody}
                onChange={(event) => setCommentBody(event.target.value)}
                placeholder={t("content.collaboration.commentPlaceholder")}
              />
              <Button
                type="button"
                onClick={() => void submitComment()}
                disabled={!commentBody.trim() || isSubmittingComment}
              >
                {isSubmittingComment ? (
                  <Loader2 className="mr-2 size-4 animate-spin" />
                ) : (
                  <MessageSquareText className="mr-2 size-4" />
                )}
                {t("content.collaboration.addComment")}
              </Button>
            </div>
            <TimelineEmpty visible={!comments.length}>
              {t("content.collaboration.noComments")}
            </TimelineEmpty>
            {comments.map((comment) => (
              <div key={comment.id} className="rounded-md border p-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="text-sm font-medium">
                    {comment.author_username || comment.author_email}
                  </span>
                  <div className="flex items-center gap-2">
                    <Badge
                      variant={
                        comment.status === "resolved" ? "outline" : "secondary"
                      }
                    >
                      {t(
                        `content.collaboration.commentStatus.${comment.status}`,
                      )}
                    </Badge>
                    <span className="text-xs text-muted-foreground">
                      {formatDashboardDate(comment.created_at, locale)}
                    </span>
                  </div>
                </div>
                <p className="mt-2 whitespace-pre-wrap text-sm">
                  {comment.body}
                </p>
                {canEdit && comment.status === "open" ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="mt-2"
                    onClick={() => void resolveComment(comment.id)}
                  >
                    <Check className="mr-2 size-4" />
                    {t("content.collaboration.resolve")}
                  </Button>
                ) : null}
              </div>
            ))}
          </TabsContent>

          <TabsContent value="activity" className="mt-4 space-y-3">
            <TimelineEmpty visible={!activities.length}>
              {t("content.collaboration.noActivity")}
            </TimelineEmpty>
            {activities.map((activity) => (
              <TimelineRow
                key={activity.id}
                title={projectActivityTitle(t, activity)}
                subtitle={projectActivityDetail(t, activity)}
                timestamp={formatDashboardDate(activity.created_at, locale)}
              />
            ))}
          </TabsContent>

          <TabsContent value="versions" className="mt-4 space-y-3">
            <TimelineEmpty visible={!versions.length}>
              {t("content.collaboration.noVersions")}
            </TimelineEmpty>
            {versions.map((version) => (
              <TimelineRow
                key={version.id}
                title={t("content.collaboration.versionTitle", {
                  number: version.version_number,
                })}
                subtitle={version.title}
                timestamp={formatDashboardDate(version.created_at, locale)}
                action={
                  canEdit ? (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => void restoreVersion(version)}
                    >
                      <RotateCcw className="mr-2 size-4" />
                      {t("content.collaboration.restore")}
                    </Button>
                  ) : null
                }
              />
            ))}
          </TabsContent>

          <TabsContent value="links" className="mt-4 space-y-3">
            <Button
              type="button"
              onClick={() => void createShareLink()}
              disabled={isCreatingShareLink}
            >
              {isCreatingShareLink ? (
                <Loader2 className="mr-2 size-4 animate-spin" />
              ) : (
                <Link2 className="mr-2 size-4" />
              )}
              {t("content.collaboration.createViewerLink")}
            </Button>
            <TimelineEmpty visible={!shareLinks.length}>
              {t("content.collaboration.noShareLinks")}
            </TimelineEmpty>
            {shareLinks.map((link) => (
              <TimelineRow
                key={link.id}
                title={t("content.collaboration.shareLinkTitle")}
                subtitle={t(
                  `content.collaboration.shareLinkStatus.${link.status}`,
                )}
                timestamp={formatDashboardDate(link.created_at, locale)}
                action={
                  link.status === "active" ? (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => void revokeShareLink(link.id)}
                    >
                      {t("content.collaboration.revokeShareLink")}
                    </Button>
                  ) : null
                }
              />
            ))}
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  );
}

function TimelineEmpty({
  children,
  visible,
}: {
  children: ReactNode;
  visible: boolean;
}) {
  if (!visible) {
    return null;
  }
  return (
    <div className="rounded-md border border-dashed bg-muted/20 p-4 text-sm text-muted-foreground">
      {children}
    </div>
  );
}

function TimelineRow({
  action,
  subtitle,
  timestamp,
  title,
}: {
  action?: ReactNode;
  subtitle: string;
  timestamp: string;
  title: string;
}) {
  return (
    <div className="flex flex-col gap-3 rounded-md border p-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <div className="text-sm font-medium">{title}</div>
        <div className="truncate text-sm text-muted-foreground">{subtitle}</div>
        <div className="text-xs text-muted-foreground">{timestamp}</div>
      </div>
      {action ? <div className="shrink-0">{action}</div> : null}
    </div>
  );
}

function projectActivityTitle(
  t: ReturnType<typeof useTranslation>["t"],
  activity: ProjectActivity,
) {
  return t(`content.collaboration.activityEvent.${activity.event_type}`);
}

function projectActivityDetail(
  t: ReturnType<typeof useTranslation>["t"],
  activity: ProjectActivity,
) {
  const actor =
    activity.actor_username ||
    activity.actor_email ||
    t("content.collaboration.unknownActor");
  const target =
    activity.target_username ||
    activity.target_email ||
    t("content.collaboration.unknownTarget");
  const platform = metadataString(activity.metadata, "platform");

  if (platform) {
    return t("content.collaboration.activityDetail.platform", {
      actor,
      platform,
    });
  }
  if (activity.target_user_id) {
    return t("content.collaboration.activityDetail.target", {
      actor,
      target,
    });
  }
  return t("content.collaboration.activityDetail.actor", { actor });
}

function metadataString(metadata: Record<string, unknown>, key: string) {
  const value = metadata[key];
  return typeof value === "string" ? value : "";
}

"use client";

import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import {
  Activity,
  Check,
  Copy,
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
import { Input } from "@/components/ui/input";
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
  type ProjectCollaboratorRole,
  type ProjectPermissionSource,
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
  permissionSources?: ProjectPermissionSource[];
  projectId: string;
  projectRole: ProjectRole | null;
};

export function ProjectCollaborationPanel({
  canEdit,
  onVersionRestore,
  permissionSources = [],
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
  const [createdUrls, setCreatedUrls] = useState<Record<string, string>>({});
  const [shareLinkModalOpen, setShareLinkModalOpen] = useState(false);
  const [shareLinkUrl, setShareLinkUrl] = useState("");
  const [shareLinkRole, setShareLinkRole] =
    useState<ProjectCollaboratorRole>("viewer");
  const [shareLinkModalRole, setShareLinkModalRole] =
    useState<ProjectCollaboratorRole>("viewer");
  const [copied, setCopied] = useState(false);
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
      toast.success(t("content.collaboration.versionRestored"), {
        description: t("content.collaboration.versionRestoredDesc"),
      });
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

  const handleCopyShareLink = async () => {
    if (!shareLinkUrl) {
      return;
    }
    try {
      if (!navigator.clipboard) {
        throw new Error(t("content.collaboration.shareLinkCopyUnavailable"));
      }
      await navigator.clipboard.writeText(shareLinkUrl);
      setCopied(true);
      toast.success(t("content.collaboration.shareLinkCopied"));
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      toast.error(t("content.collaboration.shareLinkCopyFailed"), {
        description:
          err instanceof Error ? err.message : t("content.share.retryLater"),
      });
    }
  };

  const createShareLink = async () => {
    setIsCreatingShareLink(true);
    try {
      const link = await createProjectShareLink(projectId, {
        role: shareLinkRole,
      });
      setCreatedUrls((prev) => ({ ...prev, [link.id]: link.url }));
      setShareLinks((items) => [link, ...items]);
      void loadAll();

      setShareLinkUrl(link.url);
      setShareLinkModalRole(link.role);
      setShareLinkModalOpen(true);

      try {
        if (navigator.clipboard) {
          await navigator.clipboard.writeText(link.url);
          toast.success(t("content.collaboration.shareLinkCreated"));
        }
      } catch (copyError) {
        console.warn("Failed to copy link automatically:", copyError);
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
    <>
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
          {permissionSources.length > 0 ? (
            <div className="mb-4 flex flex-wrap gap-2">
              {permissionSources.map((source, index) => (
                <Badge
                  key={`${source.source}-${source.role}-${index}`}
                  variant="outline"
                >
                  {t(
                    `content.collaboration.permissionSource.${source.source}`,
                    {
                      defaultValue: source.source,
                    },
                  )}
                  {" - "}
                  {t(`content.header.role.${source.role}`, {
                    defaultValue: source.role,
                  })}
                </Badge>
              ))}
            </div>
          ) : null}
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
                          comment.status === "resolved"
                            ? "outline"
                            : "secondary"
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
              <div className="space-y-3 rounded-md border p-3">
                <div className="grid gap-2 sm:grid-cols-2">
                  {(
                    ["viewer", "editor"] satisfies ProjectCollaboratorRole[]
                  ).map((role) => {
                    const selected = shareLinkRole === role;
                    return (
                      <button
                        key={role}
                        type="button"
                        aria-pressed={selected}
                        className={`rounded-md border p-3 text-left text-sm transition-colors ${
                          selected
                            ? "border-primary bg-primary/5 text-foreground"
                            : "bg-background text-muted-foreground hover:border-primary/60 hover:text-foreground"
                        }`}
                        onClick={() => setShareLinkRole(role)}
                      >
                        <span className="block font-medium text-foreground">
                          {shareLinkRoleLabel(t, role)}
                        </span>
                        <span className="mt-1 block text-xs">
                          {t(`content.collaboration.shareLinkRoleHelp.${role}`)}
                        </span>
                      </button>
                    );
                  })}
                </div>
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
                  {t("content.collaboration.createShareLink")}
                </Button>
              </div>
              <TimelineEmpty visible={!shareLinks.length}>
                {t("content.collaboration.noShareLinks")}
              </TimelineEmpty>
              {shareLinks.map((link) => (
                <TimelineRow
                  key={link.id}
                  title={
                    link.status === "active" ? (
                      <Button
                        type="button"
                        variant="link"
                        className="h-auto p-0 text-sm font-medium text-primary hover:underline"
                        onClick={() => {
                          const url = createdUrls[link.id] || "";
                          setShareLinkUrl(url);
                          setShareLinkModalRole(link.role);
                          setShareLinkModalOpen(true);
                        }}
                      >
                        {t("content.collaboration.shareLinkTitle")}
                      </Button>
                    ) : (
                      t("content.collaboration.shareLinkTitle")
                    )
                  }
                  subtitle={t("content.collaboration.shareLinkSubtitle", {
                    role: shareLinkRoleLabel(t, link.role),
                    status: t(
                      `content.collaboration.shareLinkStatus.${link.status}`,
                    ),
                  })}
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

      <DialogPrimitive.Root
        open={shareLinkModalOpen}
        onOpenChange={setShareLinkModalOpen}
      >
        <DialogPrimitive.Portal>
          <DialogPrimitive.Backdrop className="fixed inset-0 z-50 bg-black/20 transition-opacity duration-150 data-ending-style:opacity-0 data-starting-style:opacity-0 supports-backdrop-filter:backdrop-blur-xs" />
          <DialogPrimitive.Popup className="fixed top-1/2 left-1/2 z-50 grid w-[calc(100vw-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border bg-popover p-5 text-popover-foreground shadow-xl transition duration-150 data-ending-style:scale-95 data-ending-style:opacity-0 data-starting-style:scale-95 data-starting-style:opacity-0">
            <div className="flex flex-col gap-4">
              <DialogPrimitive.Title className="font-heading text-lg font-semibold leading-none tracking-tight">
                {t("content.collaboration.shareLinkTitle")}
              </DialogPrimitive.Title>
              <div className="rounded-md bg-muted/50 px-3 py-2 text-sm">
                <span className="text-muted-foreground">
                  {t("content.collaboration.shareLinkModalRole")}
                </span>{" "}
                <span className="font-medium">
                  {shareLinkRoleLabel(t, shareLinkModalRole)}
                </span>
              </div>

              {shareLinkUrl ? (
                <>
                  <DialogPrimitive.Description className="text-sm text-muted-foreground">
                    {t("content.collaboration.shareLinkModalDesc")}
                  </DialogPrimitive.Description>
                  <div className="flex items-center gap-2">
                    <Input readOnly value={shareLinkUrl} className="flex-1" />
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      onClick={() => void handleCopyShareLink()}
                    >
                      {copied ? (
                        <Check className="size-4 text-green-600" />
                      ) : (
                        <Copy className="size-4" />
                      )}
                    </Button>
                  </div>
                </>
              ) : (
                <DialogPrimitive.Description className="text-sm text-muted-foreground">
                  {t("content.collaboration.historyLinkNoUrlDesc")}
                </DialogPrimitive.Description>
              )}

              <div className="mt-2 flex justify-end">
                <DialogPrimitive.Close
                  render={
                    <Button type="button" variant="outline">
                      {t("content.collaboration.close")}
                    </Button>
                  }
                />
              </div>
            </div>
          </DialogPrimitive.Popup>
        </DialogPrimitive.Portal>
      </DialogPrimitive.Root>
    </>
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
  title: ReactNode;
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

function shareLinkRoleLabel(
  t: ReturnType<typeof useTranslation>["t"],
  role: ProjectCollaboratorRole,
) {
  return t(`content.collaboration.shareLinkRole.${role}`);
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

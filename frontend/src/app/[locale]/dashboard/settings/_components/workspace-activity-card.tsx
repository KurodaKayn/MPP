"use client";

import { type ComponentType, type ReactNode, useEffect, useState } from "react";
import {
  Activity,
  Building2,
  Clock3,
  History,
  PencilLine,
  RefreshCw,
  ShieldCheck,
  UserCog,
  UserMinus,
  UserPlus,
  UsersRound,
} from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  getWorkspaceActivities,
  type WorkspaceActivity,
  type WorkspaceActivityType,
  type WorkspaceRole,
} from "@/lib/dashboard/api";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import { cn } from "@/lib/utils";
import { formatDashboardDate } from "../../_lib/formatters";
import { useDashboardWorkspaceSelection } from "../../_hooks/use-dashboard-workspace-selection";
import { canManageWorkspaceMembers } from "../_lib/workspace-members";

const workspaceActivityLimit = 20;

type DashboardT = (key: string, options?: Record<string, unknown>) => string;

type ActivityIconConfig = {
  className: string;
  icon: ComponentType<{ className?: string }>;
};

export function WorkspaceActivityCard() {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const workspaceSelection = useDashboardWorkspaceSelection();
  const selectedWorkspace = workspaceSelection.selectedWorkspace;
  const selectedWorkspaceId = selectedWorkspace?.id ?? "";
  const canManage = canManageWorkspaceMembers(selectedWorkspace?.role);

  const [activities, setActivities] = useState<WorkspaceActivity[]>([]);
  const [isLoadingActivities, setIsLoadingActivities] = useState(false);

  useEffect(() => {
    let cancelled = false;

    async function loadActivities() {
      if (!selectedWorkspaceId || !canManage) {
        setActivities([]);
        setIsLoadingActivities(false);
        return;
      }

      setIsLoadingActivities(true);
      setActivities([]);
      try {
        const response = await getWorkspaceActivities(
          selectedWorkspaceId,
          workspaceActivityLimit,
        );
        if (!cancelled) {
          setActivities(response.items);
        }
      } catch (error) {
        if (!cancelled) {
          toast.error(t("settings.workspaceActivity.loadFailed"), {
            description:
              error instanceof Error
                ? error.message
                : t("settings.workspaceActivity.retryLater"),
          });
        }
      } finally {
        if (!cancelled) {
          setIsLoadingActivities(false);
        }
      }
    }

    void loadActivities();

    return () => {
      cancelled = true;
    };
  }, [canManage, selectedWorkspaceId, t]);

  const handleRefresh = async () => {
    if (!selectedWorkspaceId || !canManage) {
      return;
    }

    setIsLoadingActivities(true);
    try {
      const response = await getWorkspaceActivities(
        selectedWorkspaceId,
        workspaceActivityLimit,
      );
      setActivities(response.items);
    } catch (error) {
      toast.error(t("settings.workspaceActivity.loadFailed"), {
        description:
          error instanceof Error
            ? error.message
            : t("settings.workspaceActivity.retryLater"),
      });
    } finally {
      setIsLoadingActivities(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>{t("settings.workspaceActivity.title")}</CardTitle>
          <CardDescription>
            {t("settings.workspaceActivity.description")}
          </CardDescription>
        </div>
        {selectedWorkspaceId && canManage ? (
          <CardAction>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={isLoadingActivities}
              onClick={() => void handleRefresh()}
            >
              <RefreshCw
                className={cn(
                  "size-3.5",
                  isLoadingActivities && "animate-spin",
                )}
              />
              {t("settings.workspaceActivity.refresh")}
            </Button>
          </CardAction>
        ) : null}
      </CardHeader>
      <CardContent>
        {workspaceSelection.isLoading ? (
          <WorkspaceActivityLoading />
        ) : workspaceSelection.error ? (
          <WorkspaceActivityState
            actionLabel={t("workspace.error.retry")}
            icon={<UsersRound className="size-5" />}
            message={workspaceSelection.error}
            onAction={() => void workspaceSelection.reloadWorkspaces()}
            title={t("workspace.error.title")}
          />
        ) : !selectedWorkspace ? (
          <WorkspaceActivityState
            icon={<History className="size-5" />}
            message={t("settings.workspaceActivity.noWorkspace")}
          />
        ) : !canManage ? (
          <WorkspaceActivityState
            icon={<ShieldCheck className="size-5" />}
            message={t("settings.workspaceActivity.noPermission")}
          />
        ) : isLoadingActivities ? (
          <WorkspaceActivityLoading />
        ) : activities.length === 0 ? (
          <WorkspaceActivityState
            icon={<Activity className="size-5" />}
            message={t("settings.workspaceActivity.empty")}
          />
        ) : (
          <div className="divide-y rounded-lg border">
            {activities.map((activity) => (
              <WorkspaceActivityRow
                key={activity.id}
                activity={activity}
                locale={locale}
                t={t}
              />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function WorkspaceActivityRow({
  activity,
  locale,
  t,
}: {
  activity: WorkspaceActivity;
  locale: string;
  t: DashboardT;
}) {
  const iconConfig = workspaceActivityIcon(activity.event_type);
  const Icon = iconConfig.icon;
  const badges = workspaceActivityBadges(t, activity);

  return (
    <div className="flex items-start gap-3 p-3">
      <div
        className={cn(
          "mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg",
          iconConfig.className,
        )}
      >
        <Icon className="size-4" />
      </div>
      <div className="min-w-0 flex-1 space-y-2">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div className="min-w-0">
            <p className="truncate text-sm font-medium">
              {workspaceActivityTitle(t, activity.event_type)}
            </p>
            <p className="mt-0.5 text-sm text-muted-foreground">
              {workspaceActivityDetail(t, activity)}
            </p>
          </div>
          <time className="inline-flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
            <Clock3 className="size-3" />
            {formatDashboardDate(activity.created_at, locale)}
          </time>
        </div>

        {badges.length > 0 ? (
          <div className="flex flex-wrap gap-1.5">
            {badges.map((badge) => (
              <Badge key={badge} variant="outline">
                {badge}
              </Badge>
            ))}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function WorkspaceActivityLoading() {
  return (
    <div className="space-y-2">
      <Skeleton className="h-14 w-full" />
      <Skeleton className="h-14 w-full" />
      <Skeleton className="h-14 w-full" />
    </div>
  );
}

function WorkspaceActivityState({
  actionLabel,
  icon,
  message,
  onAction,
  title,
}: {
  actionLabel?: string;
  icon: ReactNode;
  message: string;
  onAction?: () => void;
  title?: string;
}) {
  return (
    <div className="flex min-h-32 flex-col items-center justify-center gap-3 rounded-lg border border-dashed p-5 text-center text-sm text-muted-foreground">
      <div className="flex size-10 items-center justify-center rounded-lg bg-muted">
        {icon}
      </div>
      <div>
        {title ? <p className="font-medium text-foreground">{title}</p> : null}
        <p>{message}</p>
      </div>
      {actionLabel && onAction ? (
        <Button type="button" variant="outline" onClick={onAction}>
          {actionLabel}
        </Button>
      ) : null}
    </div>
  );
}

function workspaceActivityIcon(
  eventType: WorkspaceActivityType,
): ActivityIconConfig {
  switch (eventType) {
    case "workspace_created":
      return {
        className: "bg-sky-500/10 text-sky-700 dark:text-sky-300",
        icon: Building2,
      };
    case "workspace_updated":
      return {
        className: "bg-amber-500/10 text-amber-700 dark:text-amber-300",
        icon: PencilLine,
      };
    case "member_added":
      return {
        className: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
        icon: UserPlus,
      };
    case "member_role_changed":
      return {
        className: "bg-blue-500/10 text-blue-700 dark:text-blue-300",
        icon: UserCog,
      };
    case "member_removed":
      return {
        className: "bg-rose-500/10 text-rose-700 dark:text-rose-300",
        icon: UserMinus,
      };
  }
}

function workspaceActivityTitle(
  t: DashboardT,
  eventType: WorkspaceActivityType,
) {
  return t(`settings.workspaceActivity.event.${eventType}`);
}

function workspaceActivityDetail(t: DashboardT, activity: WorkspaceActivity) {
  const actor = activityActorName(t, activity);
  const target = activityTargetName(t, activity);
  const role = workspaceRoleLabel(
    t,
    workspaceRoleFromMetadata(activity.metadata, "role"),
  );
  const previousRole = workspaceRoleLabel(
    t,
    workspaceRoleFromMetadata(activity.metadata, "previous_role"),
  );

  switch (activity.event_type) {
    case "workspace_created": {
      const workspaceName =
        metadataString(activity.metadata, "name") ??
        t("settings.workspaceActivity.unknownWorkspace");
      return t("settings.workspaceActivity.detail.workspace_created", {
        actor,
        workspace: workspaceName,
      });
    }
    case "workspace_updated": {
      const name = metadataString(activity.metadata, "name");
      const previousName = metadataString(activity.metadata, "previous_name");
      if (name && previousName && name !== previousName) {
        return t("settings.workspaceActivity.detail.workspace_updatedRenamed", {
          actor,
          name,
          previousName,
        });
      }
      return t("settings.workspaceActivity.detail.workspace_updated", {
        actor,
      });
    }
    case "member_added":
      return t("settings.workspaceActivity.detail.member_added", {
        actor,
        role,
        target,
      });
    case "member_role_changed":
      return t("settings.workspaceActivity.detail.member_role_changed", {
        actor,
        previousRole,
        role,
        target,
      });
    case "member_removed":
      return t("settings.workspaceActivity.detail.member_removed", {
        actor,
        previousRole,
        target,
      });
  }
}

function workspaceActivityBadges(t: DashboardT, activity: WorkspaceActivity) {
  const badges: string[] = [];
  const role = workspaceRoleFromMetadata(activity.metadata, "role");
  const previousRole = workspaceRoleFromMetadata(
    activity.metadata,
    "previous_role",
  );

  if (activity.event_type === "workspace_created") {
    const slug = metadataString(activity.metadata, "slug");
    if (slug) {
      badges.push(t("settings.workspaceActivity.badge.slug", { slug }));
    }
  }

  if (activity.event_type === "workspace_updated") {
    const slug = metadataString(activity.metadata, "slug");
    const previousSlug = metadataString(activity.metadata, "previous_slug");
    if (slug && previousSlug && slug !== previousSlug) {
      badges.push(
        t("settings.workspaceActivity.badge.slugChange", {
          from: previousSlug,
          to: slug,
        }),
      );
    }
  }

  if (activity.event_type === "member_added" && role) {
    badges.push(
      t("settings.workspaceActivity.badge.role", {
        role: workspaceRoleLabel(t, role),
      }),
    );
  }

  if (activity.event_type === "member_role_changed" && previousRole && role) {
    badges.push(
      t("settings.workspaceActivity.badge.roleChange", {
        from: workspaceRoleLabel(t, previousRole),
        to: workspaceRoleLabel(t, role),
      }),
    );
  }

  if (activity.event_type === "member_removed" && previousRole) {
    badges.push(
      t("settings.workspaceActivity.badge.formerRole", {
        role: workspaceRoleLabel(t, previousRole),
      }),
    );
  }

  return badges;
}

function activityActorName(t: DashboardT, activity: WorkspaceActivity) {
  return (
    activity.actor_username ||
    activity.actor_email ||
    t("settings.workspaceActivity.unknownActor")
  );
}

function activityTargetName(t: DashboardT, activity: WorkspaceActivity) {
  return (
    activity.target_username ||
    activity.target_email ||
    t("settings.workspaceActivity.unknownTarget")
  );
}

function metadataString(metadata: WorkspaceActivity["metadata"], key: string) {
  const value = metadata?.[key];
  if (typeof value !== "string") {
    return null;
  }

  const trimmed = value.trim();
  return trimmed ? trimmed : null;
}

function workspaceRoleFromMetadata(
  metadata: WorkspaceActivity["metadata"],
  key: string,
): WorkspaceRole | null {
  const value = metadataString(metadata, key);
  if (
    value === "owner" ||
    value === "admin" ||
    value === "member" ||
    value === "viewer"
  ) {
    return value;
  }
  return null;
}

function workspaceRoleLabel(t: DashboardT, role: WorkspaceRole | null) {
  return role
    ? t(`workspace.role.${role}`)
    : t("settings.workspaceActivity.unknownRole");
}

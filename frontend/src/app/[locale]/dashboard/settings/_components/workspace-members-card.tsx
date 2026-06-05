"use client";

import { type FormEvent, useEffect, useMemo, useState } from "react";
import {
  ChevronDown,
  Crown,
  Loader2,
  MailPlus,
  ShieldCheck,
  Trash2,
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
  addWorkspaceMember,
  getWorkspaceMembers,
  removeWorkspaceMember,
  updateWorkspaceMember,
  type WorkspaceMember,
  type WorkspaceRole,
} from "@/lib/dashboard/api";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import { WorkspaceSwitcher } from "../../_components/workspace-switcher";
import { useDashboardWorkspaceSelection } from "../../_hooks/use-dashboard-workspace-selection";
import {
  canManageWorkspaceMembers,
  manageableWorkspaceMemberRoles,
  type WorkspaceMemberRole,
} from "../_lib/workspace-members";

export function WorkspaceMembersCard() {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const workspaceSelection = useDashboardWorkspaceSelection();
  const selectedWorkspace = workspaceSelection.selectedWorkspace;
  const canManage = canManageWorkspaceMembers(selectedWorkspace?.role);

  const [email, setEmail] = useState("");
  const [role, setRole] = useState<WorkspaceMemberRole>("member");
  const [members, setMembers] = useState<WorkspaceMember[]>([]);
  const [isLoadingMembers, setIsLoadingMembers] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [updatingUserId, setUpdatingUserId] = useState<string | null>(null);
  const [removingUserId, setRemovingUserId] = useState<string | null>(null);

  const busyUserIds = useMemo(
    () => new Set([updatingUserId, removingUserId].filter(Boolean)),
    [removingUserId, updatingUserId],
  );
  const selectedWorkspaceId = selectedWorkspace?.id ?? "";

  useEffect(() => {
    let cancelled = false;

    async function loadMembers() {
      if (!selectedWorkspaceId || !canManage) {
        setMembers([]);
        setIsLoadingMembers(false);
        return;
      }

      setIsLoadingMembers(true);
      try {
        const response = await getWorkspaceMembers(selectedWorkspaceId);
        if (!cancelled) {
          setMembers(response.items);
        }
      } catch (error) {
        if (!cancelled) {
          toast.error(t("settings.workspaceMembers.loadFailed"), {
            description:
              error instanceof Error
                ? error.message
                : t("settings.workspaceMembers.retryLater"),
          });
        }
      } finally {
        if (!cancelled) {
          setIsLoadingMembers(false);
        }
      }
    }

    void loadMembers();

    return () => {
      cancelled = true;
    };
  }, [canManage, selectedWorkspaceId, t]);

  const handleAddMember = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmedEmail = email.trim();
    if (!selectedWorkspaceId || !canManage || !trimmedEmail) {
      return;
    }

    setIsSubmitting(true);
    try {
      const member = await addWorkspaceMember(selectedWorkspaceId, {
        email: trimmedEmail,
        role,
      });
      setMembers((items) => [
        member,
        ...items.filter((item) => item.user_id !== member.user_id),
      ]);
      setEmail("");
      toast.success(t("settings.workspaceMembers.addSuccess"));
    } catch (error) {
      toast.error(t("settings.workspaceMembers.addFailed"), {
        description:
          error instanceof Error
            ? error.message
            : t("settings.workspaceMembers.retryLater"),
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleRoleChange = async (
    member: WorkspaceMember,
    nextRole: WorkspaceMemberRole,
  ) => {
    if (!selectedWorkspaceId || member.role === nextRole) {
      return;
    }

    setUpdatingUserId(member.user_id);
    try {
      const updated = await updateWorkspaceMember(
        selectedWorkspaceId,
        member.user_id,
        { role: nextRole },
      );
      setMembers((items) =>
        items.map((item) =>
          item.user_id === updated.user_id ? updated : item,
        ),
      );
      toast.success(t("settings.workspaceMembers.roleUpdated"));
    } catch (error) {
      toast.error(t("settings.workspaceMembers.roleUpdateFailed"), {
        description:
          error instanceof Error
            ? error.message
            : t("settings.workspaceMembers.retryLater"),
      });
    } finally {
      setUpdatingUserId(null);
    }
  };

  const handleRemove = async (member: WorkspaceMember) => {
    if (!selectedWorkspaceId) {
      return;
    }

    setRemovingUserId(member.user_id);
    try {
      await removeWorkspaceMember(selectedWorkspaceId, member.user_id);
      setMembers((items) =>
        items.filter((item) => item.user_id !== member.user_id),
      );
      toast.success(t("settings.workspaceMembers.removeSuccess"));
    } catch (error) {
      toast.error(t("settings.workspaceMembers.removeFailed"), {
        description:
          error instanceof Error
            ? error.message
            : t("settings.workspaceMembers.retryLater"),
      });
    } finally {
      setRemovingUserId(null);
    }
  };

  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>{t("settings.workspaceMembers.title")}</CardTitle>
          <CardDescription>
            {t("settings.workspaceMembers.description")}
          </CardDescription>
        </div>
        <CardAction className="w-full sm:w-auto">
          <WorkspaceSwitcher
            disabled={isLoadingMembers || isSubmitting}
            isLoading={workspaceSelection.isLoading}
            selectedWorkspace={selectedWorkspace}
            workspaces={workspaceSelection.workspaces}
            onWorkspaceChange={workspaceSelection.selectWorkspace}
            onWorkspaceCreate={workspaceSelection.createWorkspace}
            isCreatingWorkspace={workspaceSelection.isCreatingWorkspace}
          />
        </CardAction>
      </CardHeader>
      <CardContent className="grid gap-4">
        {workspaceSelection.isLoading ? (
          <WorkspaceMembersLoading />
        ) : workspaceSelection.error ? (
          <WorkspaceMembersState
            actionLabel={t("workspace.error.retry")}
            icon={<UsersRound className="size-5" />}
            message={workspaceSelection.error}
            onAction={() => void workspaceSelection.reloadWorkspaces()}
            title={t("workspace.error.title")}
          />
        ) : !selectedWorkspace ? (
          <WorkspaceMembersState
            icon={<UsersRound className="size-5" />}
            message={t("settings.workspaceMembers.noWorkspace")}
          />
        ) : !canManage ? (
          <WorkspaceMembersState
            icon={<ShieldCheck className="size-5" />}
            message={t("settings.workspaceMembers.noPermission")}
          />
        ) : (
          <>
            <form
              className="grid gap-3 rounded-lg border bg-muted/20 p-3 sm:grid-cols-[minmax(0,1fr)_auto_auto] sm:items-end"
              onSubmit={handleAddMember}
            >
              <div className="space-y-2">
                <Label htmlFor="workspace-member-email">
                  {t("settings.workspaceMembers.emailLabel")}
                </Label>
                <Input
                  id="workspace-member-email"
                  type="email"
                  value={email}
                  placeholder={t("settings.workspaceMembers.emailPlaceholder")}
                  onChange={(event) => setEmail(event.target.value)}
                />
              </div>
              <WorkspaceRoleMenu
                disabled={isSubmitting}
                role={role}
                t={t}
                onRoleChange={setRole}
              />
              <Button
                type="submit"
                disabled={!email.trim() || isSubmitting || isLoadingMembers}
              >
                {isSubmitting ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <UserPlus className="size-4" />
                )}
                {t("settings.workspaceMembers.add")}
              </Button>
            </form>

            <section className="space-y-3">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <h3 className="font-medium">
                    {t("settings.workspaceMembers.people")}
                  </h3>
                  <p className="text-sm text-muted-foreground">
                    {t("settings.workspaceMembers.peopleDescription")}
                  </p>
                </div>
                <Badge variant="outline">
                  {t("settings.workspaceMembers.count", {
                    count: members.length,
                  })}
                </Badge>
              </div>

              {isLoadingMembers ? (
                <WorkspaceMembersLoading />
              ) : members.length === 0 ? (
                <WorkspaceMembersState
                  icon={<MailPlus className="size-5" />}
                  message={t("settings.workspaceMembers.empty")}
                />
              ) : (
                <div className="divide-y rounded-lg border">
                  {members.map((member) => (
                    <WorkspaceMemberRow
                      key={member.user_id}
                      busy={busyUserIds.has(member.user_id)}
                      member={member}
                      removing={removingUserId === member.user_id}
                      t={t}
                      updating={updatingUserId === member.user_id}
                      onRemove={() => void handleRemove(member)}
                      onRoleChange={(nextRole) =>
                        void handleRoleChange(member, nextRole)
                      }
                    />
                  ))}
                </div>
              )}
            </section>
          </>
        )}
      </CardContent>
    </Card>
  );
}

function WorkspaceRoleMenu({
  disabled,
  onRoleChange,
  role,
  t,
}: {
  disabled?: boolean;
  onRoleChange: (role: WorkspaceMemberRole) => void;
  role: WorkspaceMemberRole;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            type="button"
            variant="outline"
            disabled={disabled}
            className="justify-between sm:min-w-32"
          >
            {workspaceRoleLabel(t, role)}
            <ChevronDown className="size-4 text-muted-foreground" />
          </Button>
        }
      />
      <DropdownMenuContent align="end" className="w-40">
        {manageableWorkspaceMemberRoles.map((item) => (
          <DropdownMenuItem key={item} onClick={() => onRoleChange(item)}>
            <div>
              <div>{workspaceRoleLabel(t, item)}</div>
              <div className="text-xs text-muted-foreground">
                {t(`settings.workspaceMembers.roleHelp.${item}`)}
              </div>
            </div>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function WorkspaceMemberRow({
  busy,
  member,
  onRemove,
  onRoleChange,
  removing,
  t,
  updating,
}: {
  busy: boolean;
  member: WorkspaceMember;
  onRemove: () => void;
  onRoleChange: (role: WorkspaceMemberRole) => void;
  removing: boolean;
  t: (key: string, options?: Record<string, unknown>) => string;
  updating: boolean;
}) {
  const isOwner = member.role === "owner";

  return (
    <div className="flex items-center gap-3 p-3">
      <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted text-sm font-medium">
        {member.username.slice(0, 1).toUpperCase() || "U"}
      </div>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">{member.username}</p>
        <p className="truncate text-xs text-muted-foreground">{member.email}</p>
      </div>
      {isOwner ? (
        <Badge variant="secondary" className="gap-1">
          <Crown className="size-3" />
          {t("settings.workspaceMembers.ownerLocked")}
        </Badge>
      ) : (
        <>
          <WorkspaceRoleMenu
            disabled={busy}
            role={member.role as WorkspaceMemberRole}
            t={t}
            onRoleChange={onRoleChange}
          />
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            disabled={busy}
            onClick={onRemove}
          >
            {removing ? (
              <Loader2 className="size-4 animate-spin" />
            ) : updating ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Trash2 className="size-4 text-destructive" />
            )}
            <span className="sr-only">
              {t("settings.workspaceMembers.remove")}
            </span>
          </Button>
        </>
      )}
    </div>
  );
}

function WorkspaceMembersLoading() {
  return (
    <div className="space-y-2">
      <Skeleton className="h-12 w-full" />
      <Skeleton className="h-12 w-full" />
    </div>
  );
}

function WorkspaceMembersState({
  actionLabel,
  icon,
  message,
  onAction,
  title,
}: {
  actionLabel?: string;
  icon: React.ReactNode;
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

function workspaceRoleLabel(
  t: (key: string, options?: Record<string, unknown>) => string,
  role: WorkspaceRole,
) {
  return t(`workspace.role.${role}`);
}

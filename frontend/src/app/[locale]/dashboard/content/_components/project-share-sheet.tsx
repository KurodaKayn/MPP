"use client";

import { type FormEvent, useEffect, useState } from "react";
import {
  ChevronDown,
  Loader2,
  MailPlus,
  ShieldCheck,
  Trash2,
  UserPlus,
  Users,
} from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import {
  addProjectCollaborator,
  getProjectCollaborators,
  removeProjectCollaborator,
  updateProjectCollaborator,
} from "@/lib/dashboard/api";
import type {
  ProjectCollaborator,
  ProjectCollaboratorRole,
} from "@/lib/dashboard/api";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import { cn } from "@/lib/utils";

type ProjectShareSheetProps = {
  projectId: string;
};

const collaboratorRoles: ProjectCollaboratorRole[] = ["editor", "viewer"];

export function ProjectShareSheet({ projectId }: ProjectShareSheetProps) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<ProjectCollaboratorRole>("editor");
  const [collaborators, setCollaborators] = useState<ProjectCollaborator[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [updatingUserId, setUpdatingUserId] = useState<string | null>(null);
  const [removingUserId, setRemovingUserId] = useState<string | null>(null);

  useEffect(() => {
    if (!open) {
      return;
    }

    let cancelled = false;

    async function loadCollaborators() {
      setIsLoading(true);
      try {
        const response = await getProjectCollaborators(projectId);
        if (!cancelled) {
          setCollaborators(response.items);
        }
      } catch (error) {
        if (!cancelled) {
          toast.error(t("content.share.loadFailed"), {
            description:
              error instanceof Error
                ? error.message
                : t("content.share.retryLater"),
          });
        }
      } finally {
        if (!cancelled) {
          setIsLoading(false);
        }
      }
    }

    void loadCollaborators();

    return () => {
      cancelled = true;
    };
  }, [open, projectId, t]);

  const handleAddCollaborator = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();

    const trimmedEmail = email.trim();
    if (!trimmedEmail) {
      return;
    }

    setIsSubmitting(true);
    try {
      const collaborator = await addProjectCollaborator(projectId, {
        email: trimmedEmail,
        role,
      });
      setCollaborators((items) => [
        collaborator,
        ...items.filter((item) => item.user_id !== collaborator.user_id),
      ]);
      setEmail("");
      toast.success(t("content.share.addSuccess"));
    } catch (error) {
      toast.error(t("content.share.addFailed"), {
        description:
          error instanceof Error
            ? error.message
            : t("content.share.retryLater"),
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleRoleChange = async (
    collaborator: ProjectCollaborator,
    nextRole: ProjectCollaboratorRole,
  ) => {
    if (collaborator.role === nextRole) {
      return;
    }

    setUpdatingUserId(collaborator.user_id);
    try {
      const updated = await updateProjectCollaborator(
        projectId,
        collaborator.user_id,
        { role: nextRole },
      );
      setCollaborators((items) =>
        items.map((item) =>
          item.user_id === updated.user_id ? updated : item,
        ),
      );
      toast.success(t("content.share.roleUpdated"));
    } catch (error) {
      toast.error(t("content.share.roleUpdateFailed"), {
        description:
          error instanceof Error
            ? error.message
            : t("content.share.retryLater"),
      });
    } finally {
      setUpdatingUserId(null);
    }
  };

  const handleRemove = async (collaborator: ProjectCollaborator) => {
    setRemovingUserId(collaborator.user_id);
    try {
      await removeProjectCollaborator(projectId, collaborator.user_id);
      setCollaborators((items) =>
        items.filter((item) => item.user_id !== collaborator.user_id),
      );
      toast.success(t("content.share.removeSuccess"));
    } catch (error) {
      toast.error(t("content.share.removeFailed"), {
        description:
          error instanceof Error
            ? error.message
            : t("content.share.retryLater"),
      });
    } finally {
      setRemovingUserId(null);
    }
  };

  return (
    <Sheet open={open} onOpenChange={(nextOpen) => setOpen(nextOpen)}>
      <SheetTrigger
        render={
          <Button type="button" variant="outline">
            <Users className="mr-2 size-4" />
            {t("content.share.button")}
          </Button>
        }
      />
      <SheetContent className="w-full gap-0 overflow-hidden p-0 sm:max-w-md">
        <SheetHeader className="border-b bg-muted/40 px-6 py-5">
          <div className="flex items-center gap-3">
            <div className="flex size-10 items-center justify-center rounded-2xl bg-primary/10 text-primary">
              <ShieldCheck className="size-5" />
            </div>
            <div>
              <SheetTitle>{t("content.share.title")}</SheetTitle>
              <SheetDescription>
                {t("content.share.description")}
              </SheetDescription>
            </div>
          </div>
        </SheetHeader>

        <div className="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto px-6 py-5">
          <section className="rounded-2xl border bg-background p-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <p className="font-medium">{t("content.share.ownerTitle")}</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  {t("content.share.ownerDescription")}
                </p>
              </div>
              <Badge variant="secondary">{t("content.share.roleOwner")}</Badge>
            </div>
          </section>

          <form className="space-y-4" onSubmit={handleAddCollaborator}>
            <div className="space-y-2">
              <Label htmlFor="project-share-email">
                {t("content.share.emailLabel")}
              </Label>
              <div className="flex gap-2">
                <Input
                  id="project-share-email"
                  type="email"
                  value={email}
                  placeholder={t("content.share.emailPlaceholder")}
                  onChange={(event) => setEmail(event.target.value)}
                />
                <Button
                  type="submit"
                  disabled={!email.trim() || isSubmitting}
                  className="shrink-0"
                >
                  {isSubmitting ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <UserPlus className="size-4" />
                  )}
                  <span className="sr-only">{t("content.share.add")}</span>
                </Button>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-2">
              {collaboratorRoles.map((item) => (
                <RoleButton
                  key={item}
                  active={role === item}
                  label={roleLabel(t, item)}
                  description={t(`content.share.roleHelp.${item}`)}
                  onClick={() => setRole(item)}
                />
              ))}
            </div>
          </form>

          <section className="space-y-3">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h3 className="font-medium">{t("content.share.people")}</h3>
                <p className="text-sm text-muted-foreground">
                  {t("content.share.peopleDescription")}
                </p>
              </div>
              <Badge variant="outline">
                {t("content.share.count", { count: collaborators.length })}
              </Badge>
            </div>

            {isLoading ? (
              <div className="rounded-2xl border border-dashed p-5 text-sm text-muted-foreground">
                <Loader2 className="mr-2 inline size-4 animate-spin" />
                {t("content.share.loading")}
              </div>
            ) : collaborators.length === 0 ? (
              <div className="rounded-2xl border border-dashed p-5 text-sm text-muted-foreground">
                <MailPlus className="mb-3 size-5" />
                {t("content.share.empty")}
              </div>
            ) : (
              <div className="space-y-2">
                {collaborators.map((collaborator) => (
                  <CollaboratorRow
                    key={collaborator.user_id}
                    collaborator={collaborator}
                    isRemoving={removingUserId === collaborator.user_id}
                    isUpdating={updatingUserId === collaborator.user_id}
                    onRemove={() => void handleRemove(collaborator)}
                    onRoleChange={(nextRole) =>
                      void handleRoleChange(collaborator, nextRole)
                    }
                    t={t}
                  />
                ))}
              </div>
            )}
          </section>
        </div>
      </SheetContent>
    </Sheet>
  );
}

function RoleButton({
  active,
  description,
  label,
  onClick,
}: {
  active: boolean;
  description: string;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className={cn(
        "rounded-xl border p-3 text-left transition hover:border-primary/50 hover:bg-primary/5",
        active && "border-primary bg-primary/10 text-primary",
      )}
      onClick={onClick}
    >
      <span className="block text-sm font-medium">{label}</span>
      <span className="mt-1 block text-xs text-muted-foreground">
        {description}
      </span>
    </button>
  );
}

function CollaboratorRow({
  collaborator,
  isRemoving,
  isUpdating,
  onRemove,
  onRoleChange,
  t,
}: {
  collaborator: ProjectCollaborator;
  isRemoving: boolean;
  isUpdating: boolean;
  onRemove: () => void;
  onRoleChange: (role: ProjectCollaboratorRole) => void;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  const isBusy = isRemoving || isUpdating;

  return (
    <div className="flex items-center gap-3 rounded-2xl border bg-background p-3">
      <div className="flex size-9 shrink-0 items-center justify-center rounded-full bg-muted text-sm font-medium">
        {collaborator.username.slice(0, 1).toUpperCase() || "U"}
      </div>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">{collaborator.username}</p>
        <p className="truncate text-xs text-muted-foreground">
          {collaborator.email}
        </p>
      </div>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={isBusy}
              className="min-w-24 justify-between"
            >
              {isUpdating ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                roleLabel(t, collaborator.role)
              )}
              <ChevronDown className="size-3.5 text-muted-foreground" />
            </Button>
          }
        />
        <DropdownMenuContent align="end" className="w-32">
          {collaboratorRoles.map((role) => (
            <DropdownMenuItem key={role} onClick={() => onRoleChange(role)}>
              {roleLabel(t, role)}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        disabled={isBusy}
        onClick={onRemove}
      >
        {isRemoving ? (
          <Loader2 className="size-4 animate-spin" />
        ) : (
          <Trash2 className="size-4 text-destructive" />
        )}
        <span className="sr-only">{t("content.share.remove")}</span>
      </Button>
    </div>
  );
}

function roleLabel(
  t: (key: string, options?: Record<string, unknown>) => string,
  role: ProjectCollaboratorRole,
) {
  return t(`content.share.role.${role}`);
}

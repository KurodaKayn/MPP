"use client";

import { type FormEvent, useState } from "react";
import { Check, ChevronsUpDown, Loader2, Plus, UsersRound } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import type { CreateWorkspaceInput, Workspace } from "@/lib/dashboard/api";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import { cn } from "@/lib/utils";

type WorkspaceSwitcherProps = {
  className?: string;
  disabled?: boolean;
  isCreatingWorkspace?: boolean;
  isLoading?: boolean;
  onWorkspaceCreate?: (input: CreateWorkspaceInput) => Promise<Workspace>;
  onWorkspaceChange: (workspaceId: string) => void;
  selectedWorkspace: Workspace | null;
  workspaces: Workspace[];
};

export function WorkspaceSwitcher({
  className,
  disabled = false,
  isCreatingWorkspace = false,
  isLoading = false,
  onWorkspaceCreate,
  onWorkspaceChange,
  selectedWorkspace,
  workspaces,
}: WorkspaceSwitcherProps) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const [createOpen, setCreateOpen] = useState(false);
  const canCreateWorkspace = Boolean(onWorkspaceCreate);

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              type="button"
              variant="outline"
              disabled={
                disabled ||
                isLoading ||
                (workspaces.length === 0 && !canCreateWorkspace)
              }
              className={cn(
                "h-9 w-full justify-between gap-3 sm:w-64",
                className,
              )}
            >
              <span className="flex min-w-0 items-center gap-2">
                {isLoading ? (
                  <Loader2 className="size-4 animate-spin text-muted-foreground" />
                ) : (
                  <UsersRound className="size-4 text-muted-foreground" />
                )}
                <span className="truncate">
                  {selectedWorkspace?.name ?? t("workspace.switcher.empty")}
                </span>
              </span>
              <ChevronsUpDown className="size-4 shrink-0 text-muted-foreground" />
            </Button>
          }
        />
        <DropdownMenuContent align="end" className="w-64">
          <DropdownMenuGroup>
            <DropdownMenuLabel>
              {t("workspace.switcher.label")}
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            {workspaces.map((workspace) => (
              <DropdownMenuItem
                key={workspace.id}
                onClick={() => onWorkspaceChange(workspace.id)}
                className="gap-2"
              >
                <UsersRound className="size-4 text-muted-foreground" />
                <span className="min-w-0 flex-1 truncate">
                  {workspace.name}
                </span>
                <span className="text-xs text-muted-foreground">
                  {t(`workspace.role.${workspace.role}`)}
                </span>
                <Check
                  className={cn(
                    "size-4",
                    selectedWorkspace?.id === workspace.id
                      ? "opacity-100"
                      : "opacity-0",
                  )}
                />
              </DropdownMenuItem>
            ))}
            {canCreateWorkspace ? (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  className="gap-2"
                  onClick={() => setCreateOpen(true)}
                >
                  {isCreatingWorkspace ? (
                    <Loader2 className="size-4 animate-spin text-muted-foreground" />
                  ) : (
                    <Plus className="size-4 text-muted-foreground" />
                  )}
                  <span className="min-w-0 flex-1 truncate">
                    {t("workspace.switcher.create")}
                  </span>
                </DropdownMenuItem>
              </>
            ) : null}
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>

      {onWorkspaceCreate ? (
        <WorkspaceCreateSheet
          isCreatingWorkspace={isCreatingWorkspace}
          open={createOpen}
          t={t}
          onOpenChange={setCreateOpen}
          onWorkspaceCreate={onWorkspaceCreate}
        />
      ) : null}
    </>
  );
}

function WorkspaceCreateSheet({
  isCreatingWorkspace,
  onOpenChange,
  onWorkspaceCreate,
  open,
  t,
}: {
  isCreatingWorkspace: boolean;
  onOpenChange: (open: boolean) => void;
  onWorkspaceCreate: (input: CreateWorkspaceInput) => Promise<Workspace>;
  open: boolean;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");

  const handleCreateWorkspace = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmedName = name.trim();
    const trimmedSlug = slug.trim();
    if (!trimmedName || isCreatingWorkspace) {
      return;
    }

    try {
      await onWorkspaceCreate({
        name: trimmedName,
        ...(trimmedSlug ? { slug: trimmedSlug } : {}),
      });
      setName("");
      setSlug("");
      onOpenChange(false);
      toast.success(t("workspace.switcher.createSuccess"));
    } catch (error) {
      toast.error(t("workspace.switcher.createFailed"), {
        description:
          error instanceof Error
            ? error.message
            : t("workspace.switcher.retryLater"),
      });
    }
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:max-w-md">
        <SheetHeader>
          <SheetTitle>{t("workspace.switcher.createTitle")}</SheetTitle>
          <SheetDescription>
            {t("workspace.switcher.createDescription")}
          </SheetDescription>
        </SheetHeader>
        <form className="grid gap-4 px-4" onSubmit={handleCreateWorkspace}>
          <div className="space-y-2">
            <Label htmlFor="workspace-name">
              {t("workspace.switcher.nameLabel")}
            </Label>
            <Input
              id="workspace-name"
              value={name}
              placeholder={t("workspace.switcher.namePlaceholder")}
              onChange={(event) => setName(event.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="workspace-slug">
              {t("workspace.switcher.slugLabel")}
            </Label>
            <Input
              id="workspace-slug"
              value={slug}
              placeholder={t("workspace.switcher.slugPlaceholder")}
              onChange={(event) => setSlug(event.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              {t("workspace.switcher.slugHelp")}
            </p>
          </div>
          <SheetFooter className="px-0">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("workspace.switcher.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={!name.trim() || isCreatingWorkspace}
            >
              {isCreatingWorkspace ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Plus className="size-4" />
              )}
              {t("workspace.switcher.createSubmit")}
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  );
}

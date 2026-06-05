"use client";

import { Check, ChevronsUpDown, Loader2, UsersRound } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { Workspace } from "@/lib/dashboard/api";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import { cn } from "@/lib/utils";

type WorkspaceSwitcherProps = {
  disabled?: boolean;
  isLoading?: boolean;
  onWorkspaceChange: (workspaceId: string) => void;
  selectedWorkspace: Workspace | null;
  workspaces: Workspace[];
};

export function WorkspaceSwitcher({
  disabled = false,
  isLoading = false,
  onWorkspaceChange,
  selectedWorkspace,
  workspaces,
}: WorkspaceSwitcherProps) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            type="button"
            variant="outline"
            disabled={disabled || isLoading || workspaces.length === 0}
            className="h-9 w-full justify-between gap-3 sm:w-64"
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
        <DropdownMenuLabel>{t("workspace.switcher.label")}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {workspaces.map((workspace) => (
          <DropdownMenuItem
            key={workspace.id}
            onClick={() => onWorkspaceChange(workspace.id)}
            className="gap-2"
          >
            <UsersRound className="size-4 text-muted-foreground" />
            <span className="min-w-0 flex-1 truncate">{workspace.name}</span>
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
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

"use client";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type { ProjectRole } from "@/lib/dashboard/api";
import { Loader2, Save, Send } from "lucide-react";
import { useTranslation, useAppLocale } from "@/lib/i18n/client";
import { ProjectShareSheet } from "./project-share-sheet";

type ContentPageHeaderProps = {
  canSave?: boolean;
  isSaving?: boolean;
  mode?: "create" | "edit";
  onOpenPublishPanel: () => void;
  onSave?: () => void;
  projectId?: string;
  projectRole?: ProjectRole | null;
};

export function ContentPageHeader({
  canSave = false,
  isSaving = false,
  mode = "create",
  onOpenPublishPanel,
  onSave,
  projectId,
  projectRole,
}: ContentPageHeaderProps) {
  const isEditing = mode === "edit";
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const canShare = Boolean(projectId && projectRole === "owner");

  return (
    <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <div className="flex flex-wrap items-center gap-3">
          <h2 className="text-3xl font-bold tracking-tight">
            {isEditing
              ? t("content.header.titleEdit")
              : t("content.header.titleCreate")}
          </h2>
          {isEditing && projectRole ? (
            <Badge variant="secondary">
              {t(`content.header.role.${projectRole}`)}
            </Badge>
          ) : null}
        </div>
        <p className="text-muted-foreground">
          {isEditing
            ? t("content.header.descEdit")
            : t("content.header.descCreate")}
        </p>
      </div>
      <div className="flex flex-wrap gap-2">
        {canShare && projectId ? (
          <ProjectShareSheet projectId={projectId} />
        ) : null}
        {onSave ? (
          <Button
            type="button"
            variant="outline"
            onClick={onSave}
            disabled={!canSave || isSaving}
          >
            {isSaving ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Save className="mr-2 h-4 w-4" />
            )}
            {t("content.header.saveChanges")}
          </Button>
        ) : null}
        <Button onClick={onOpenPublishPanel}>
          <Send className="mr-2 h-4 w-4" />{" "}
          {t("content.header.publishSettings")}
        </Button>
      </div>
    </div>
  );
}

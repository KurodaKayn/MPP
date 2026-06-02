"use client";

import { Button } from "@/components/ui/button";
import { Loader2, Save, Send } from "lucide-react";
import { useTranslation, useAppLocale } from "@/lib/i18n/client";

type ContentPageHeaderProps = {
  canSave?: boolean;
  isSaving?: boolean;
  mode?: "create" | "edit";
  onOpenPublishPanel: () => void;
  onSave?: () => void;
};

export function ContentPageHeader({
  canSave = false,
  isSaving = false,
  mode = "create",
  onOpenPublishPanel,
  onSave,
}: ContentPageHeaderProps) {
  const isEditing = mode === "edit";
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");

  return (
    <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h2 className="text-3xl font-bold tracking-tight">
          {isEditing
            ? t("content.header.titleEdit")
            : t("content.header.titleCreate")}
        </h2>
        <p className="text-muted-foreground">
          {isEditing
            ? t("content.header.descEdit")
            : t("content.header.descCreate")}
        </p>
      </div>
      <div className="flex flex-wrap gap-2">
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

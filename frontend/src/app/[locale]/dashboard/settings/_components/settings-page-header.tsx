"use client";

import { useParams } from "next/navigation";
import { useTranslation } from "@/lib/i18n/client";

export function SettingsPageHeader() {
  const params = useParams();
  const locale = (params?.locale as string) || "en";
  const { t } = useTranslation(locale, "dashboard");

  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">
          {t("settings.title")}
        </h1>
        <p className="text-muted-foreground mt-1">
          {t("settings.description")}
        </p>
      </div>
    </div>
  );
}

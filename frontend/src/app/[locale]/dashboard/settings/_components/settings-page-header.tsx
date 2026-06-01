"use client";

import { useTranslation, useAppLocale } from "@/lib/i18n/client";

export function SettingsPageHeader() {
  const locale = useAppLocale();
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

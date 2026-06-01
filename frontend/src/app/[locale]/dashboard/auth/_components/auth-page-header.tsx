"use client";

import { type ComponentProps } from "react";

import { Badge } from "@/components/ui/badge";
import { useTranslation, useAppLocale } from "@/lib/i18n/client";

type AccountStatus = "unconfigured" | "untested" | "connected" | "failed";

const statusVariant: Record<
  AccountStatus,
  ComponentProps<typeof Badge>["variant"]
> = {
  connected: "default",
  failed: "destructive",
  unconfigured: "outline",
  untested: "secondary",
};

export function AuthPageHeader({
  connectedCount,
  status,
  totalCount,
}: {
  connectedCount: number;
  status: AccountStatus;
  totalCount: number;
}) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");

  const statusLabel: Record<AccountStatus, string> = {
    connected: t("auth.status.connected"),
    failed: t("auth.status.failed"),
    unconfigured: t("auth.status.unconnected"),
    untested: t("auth.status.untested"),
  };

  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <p className="text-muted-foreground">{t("auth.header.description")}</p>
      </div>
      <div className="flex items-center gap-2">
        <Badge variant="outline">
          {connectedCount}/{totalCount} {t("auth.header.connectedSuffix")}
        </Badge>
        <Badge variant={statusVariant[status]}>{statusLabel[status]}</Badge>
      </div>
    </div>
  );
}

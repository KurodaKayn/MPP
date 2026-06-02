"use client";

import { BadgeCheck, Loader2, RadioTower, RotateCw } from "lucide-react";

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
import { type DouyinAccount } from "@/lib/dashboard/api";
import { useTranslation, useAppLocale } from "@/lib/i18n/client";
import { getIntlLocale } from "@/lib/i18n/settings";

type DouyinAccountCardProps = {
  account: DouyinAccount | null;
  connecting: boolean;
  loading: boolean;
  onConnect: () => void;
};

export function DouyinAccountCard({
  account,
  connecting,
  loading,
  onConnect,
}: DouyinAccountCardProps) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");

  const connected = account?.status === "connected";
  const disabled = loading || connecting;

  function formatUpdatedAt(value?: string) {
    if (!value) {
      return t("auth.douyin.unconnected");
    }

    return new Intl.DateTimeFormat(getIntlLocale(locale), {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(value));
  }

  function statusLabel(status?: DouyinAccount["status"]) {
    switch (status) {
      case "connected":
        return t("auth.status.connected");
      case "failed":
        return t("auth.status.failed");
      case "untested":
        return t("auth.status.untested");
      default:
        return t("auth.status.unconnected");
    }
  }

  return (
    <Card className="overflow-hidden border-orange-200/70 bg-gradient-to-br from-orange-50 via-background to-background">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <RadioTower className="size-4 text-orange-600" />
          {t("auth.douyin.title")}
        </CardTitle>
        <CardDescription>{t("auth.douyin.description")}</CardDescription>
        <CardAction>
          <Badge variant={connected ? "default" : "outline"}>
            {statusLabel(account?.status)}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="flex min-h-[220px] items-center">
        <div className="w-full rounded-xl border bg-background/80 p-4 shadow-sm">
          <div className="flex items-center justify-between gap-4">
            <div className="flex min-w-0 items-center gap-3">
              <div className="flex size-10 shrink-0 items-center justify-center rounded-full bg-orange-100 text-orange-700">
                <BadgeCheck className="size-5" />
              </div>
              <div className="min-w-0">
                <p className="font-medium">
                  {connected
                    ? account?.username || "Connected Douyin account"
                    : t("auth.douyin.notConnectedUsername")}
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  {t("auth.douyin.lastUpdated")}:{" "}
                  {formatUpdatedAt(account?.updated_at)}
                </p>
                {account?.last_test_error ? (
                  <p className="mt-2 text-sm text-destructive">
                    {account.last_test_error}
                  </p>
                ) : null}
              </div>
            </div>
            <Button
              type="button"
              onClick={onConnect}
              disabled={disabled}
              className="shrink-0"
            >
              {connecting ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <RotateCw className="size-4" />
              )}
              {connected
                ? t("auth.douyin.reconnect")
                : t("auth.douyin.connect")}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

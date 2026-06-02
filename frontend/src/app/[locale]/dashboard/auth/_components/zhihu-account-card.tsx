"use client";

import { BadgeCheck, BookOpen, Loader2, RotateCw } from "lucide-react";

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
import { type ZhihuAccount } from "@/lib/dashboard/api";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";

type ZhihuAccountCardProps = {
  account: ZhihuAccount | null;
  connecting: boolean;
  loading: boolean;
  onConnect: () => void;
};

export function ZhihuAccountCard({
  account,
  connecting,
  loading,
  onConnect,
}: ZhihuAccountCardProps) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const connected = account?.status === "connected";
  const disabled = loading || connecting;

  const formatUpdatedAt = (value?: string) => {
    if (!value) {
      return t("auth.status.notConnected");
    }

    const date = new Intl.DateTimeFormat(locale, {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(value));

    return t("auth.zhihu.lastUpdated", { date });
  };

  const statusLabel = (status?: ZhihuAccount["status"]) => {
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
  };

  return (
    <Card className="overflow-hidden border-blue-200/70 bg-gradient-to-br from-blue-50 via-background to-background">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <BookOpen className="size-4 text-blue-600" />
          {t("auth.zhihu.title")}
        </CardTitle>
        <CardDescription>{t("auth.zhihu.description")}</CardDescription>
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
              <div className="flex size-10 shrink-0 items-center justify-center rounded-full bg-blue-100 text-blue-700">
                <BadgeCheck className="size-5" />
              </div>
              <div className="min-w-0">
                <p className="font-medium">
                  {connected
                    ? account?.username || "Connected Zhihu account"
                    : t("auth.zhihu.notConnectedUsername")}
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
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
              {connected ? t("auth.zhihu.reconnect") : t("auth.zhihu.connect")}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

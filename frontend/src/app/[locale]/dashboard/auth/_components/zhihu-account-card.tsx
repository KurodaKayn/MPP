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

type ZhihuAccountCardProps = {
  account: ZhihuAccount | null;
  connecting: boolean;
  loading: boolean;
  onConnect: () => void;
};

function formatUpdatedAt(value?: string) {
  if (!value) {
    return "尚未连接";
  }

  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function statusLabel(status?: ZhihuAccount["status"]) {
  switch (status) {
    case "connected":
      return "已连接";
    case "failed":
      return "连接失效";
    case "untested":
      return "待验证";
    default:
      return "未连接";
  }
}

export function ZhihuAccountCard({
  account,
  connecting,
  loading,
  onConnect,
}: ZhihuAccountCardProps) {
  const connected = account?.status === "connected";
  const disabled = loading || connecting;

  return (
    <Card className="overflow-hidden border-blue-200/70 bg-gradient-to-br from-blue-50 via-background to-background">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <BookOpen className="size-4 text-blue-600" />
          知乎创作者中心
        </CardTitle>
        <CardDescription>
          在隔离浏览器中扫码或登录，MPP 只保存发布所需 Cookie。
        </CardDescription>
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
                    : "尚未连接知乎账号"}
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  最近更新：{formatUpdatedAt(account?.updated_at)}
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
              {connected ? "重新连接" : "连接知乎"}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

"use client";

import { type ComponentProps } from "react";

import { Badge } from "@/components/ui/badge";

type AccountStatus = "unconfigured" | "untested" | "connected" | "failed";

const statusLabel: Record<AccountStatus, string> = {
  connected: "已连接",
  failed: "测试失败",
  unconfigured: "未配置",
  untested: "待测试",
};

const statusVariant: Record<
  AccountStatus,
  ComponentProps<typeof Badge>["variant"]
> = {
  connected: "default",
  failed: "destructive",
  unconfigured: "outline",
  untested: "secondary",
};

export function SettingsPageHeader({
  connectedCount,
  status,
  totalCount,
}: {
  connectedCount: number;
  status: AccountStatus;
  totalCount: number;
}) {
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <p className="text-muted-foreground">
          管理平台接口凭证和发布前置条件。
        </p>
      </div>
      <div className="flex items-center gap-2">
        <Badge variant="outline">
          {connectedCount}/{totalCount} 已连接
        </Badge>
        <Badge variant={statusVariant[status]}>{statusLabel[status]}</Badge>
      </div>
    </div>
  );
}

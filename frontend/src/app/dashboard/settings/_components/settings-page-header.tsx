"use client";

import { type ComponentProps } from "react";

import { Badge } from "@/components/ui/badge";
import { type WechatAccount } from "@/lib/dashboard/api";

const statusLabel: Record<WechatAccount["status"], string> = {
  connected: "已连接",
  failed: "测试失败",
  unconfigured: "未配置",
  untested: "待测试",
};

const statusVariant: Record<
  WechatAccount["status"],
  ComponentProps<typeof Badge>["variant"]
> = {
  connected: "default",
  failed: "destructive",
  unconfigured: "outline",
  untested: "secondary",
};

export function SettingsPageHeader({
  status,
}: {
  status: WechatAccount["status"];
}) {
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <h2 className="text-3xl font-bold">设置</h2>
        <p className="text-muted-foreground">
          管理微信公众号接口凭证和发布前置条件。
        </p>
      </div>
      <Badge variant={statusVariant[status]}>{statusLabel[status]}</Badge>
    </div>
  );
}

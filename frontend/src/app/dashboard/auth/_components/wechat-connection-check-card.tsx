"use client";

import {
  AlertTriangle,
  CheckCircle2,
  CircleDashed,
  ShieldCheck,
} from "lucide-react";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { type RequirementStatus } from "@/lib/dashboard/api";
import { cn } from "@/lib/utils";

type WechatConnectionCheckCardProps = {
  lastTestedAt?: string;
  ipHint?: RequirementStatus;
  authHint?: RequirementStatus;
  testError?: string;
  errCode?: number;
  errMsg?: string;
};

function formatDate(value?: string) {
  if (!value) {
    return "尚未测试";
  }

  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function hintStyle(status: RequirementStatus["status"]) {
  switch (status) {
    case "passed":
      return {
        className: "border-emerald-200 bg-emerald-50 text-emerald-950",
        icon: CheckCircle2,
      };
    case "failed":
      return {
        className: "border-destructive/25 bg-destructive/10 text-destructive",
        icon: AlertTriangle,
      };
    case "warning":
      return {
        className: "border-amber-200 bg-amber-50 text-amber-950",
        icon: AlertTriangle,
      };
    default:
      return {
        className: "border-border bg-muted/40 text-muted-foreground",
        icon: CircleDashed,
      };
  }
}

function RequirementPanel({ hint }: { hint: RequirementStatus }) {
  const style = hintStyle(hint.status);
  const Icon = style.icon;

  return (
    <div className={cn("rounded-lg border p-4", style.className)}>
      <div className="flex items-start gap-3">
        <Icon className="mt-0.5 size-4 shrink-0" />
        <div className="min-w-0 space-y-1">
          <div className="text-sm font-medium">{hint.title}</div>
          <p className="text-sm leading-6 opacity-90">{hint.message}</p>
        </div>
      </div>
    </div>
  );
}

export function WechatConnectionCheckCard({
  lastTestedAt,
  ipHint,
  authHint,
  testError,
  errCode,
  errMsg,
}: WechatConnectionCheckCardProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheck className="size-4" />
          连接检查
        </CardTitle>
        <CardDescription>最近测试：{formatDate(lastTestedAt)}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="grid gap-3">
          {ipHint ? <RequirementPanel hint={ipHint} /> : null}
          {authHint ? <RequirementPanel hint={authHint} /> : null}
          {testError ? (
            <div className="rounded-lg border border-destructive/20 bg-destructive/10 p-4 text-sm leading-6 text-destructive">
              {testError}
            </div>
          ) : null}
          {errCode ? (
            <div className="rounded-lg border bg-muted/40 p-4 text-sm text-muted-foreground">
              微信错误码：{errCode}
              {errMsg ? ` / ${errMsg}` : ""}
            </div>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}

"use client";

import { AlertTriangle, CheckCircle2, Loader2, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { type BrowserSessionStatus } from "@/lib/dashboard/api";

type RemoteBrowserSessionModalProps = {
  completing: boolean;
  error?: string;
  expiresAt?: string;
  platformLabel: string;
  status: BrowserSessionStatus;
  streamURL?: string;
  onCancel: () => void;
  onComplete: () => void;
};

const statusText: Record<BrowserSessionStatus, string> = {
  capturing: "正在验证登录",
  connected: "已连接",
  expired: "已过期",
  failed: "连接失败",
  login_detected: "检测到登录",
  pending: "正在启动",
  ready: "等待登录",
};

function formatCountdown(value?: string) {
  if (!value) {
    return "15 分钟内完成";
  }

  const ms = new Date(value).valueOf() - Date.now();
  if (!Number.isFinite(ms) || ms <= 0) {
    return "即将过期";
  }

  const minutes = Math.floor(ms / 60000);
  const seconds = Math.floor((ms % 60000) / 1000);
  return `${minutes}:${String(seconds).padStart(2, "0")}`;
}

export function RemoteBrowserSessionModal({
  completing,
  error,
  expiresAt,
  platformLabel,
  status,
  streamURL,
  onCancel,
  onComplete,
}: RemoteBrowserSessionModalProps) {
  const canComplete =
    status === "ready" || status === "login_detected" || status === "capturing";

  return (
    <div className="fixed inset-0 z-50 bg-background/85 p-3 backdrop-blur-sm sm:p-6">
      <div className="mx-auto flex h-full max-w-7xl flex-col overflow-hidden rounded-2xl border bg-background shadow-2xl">
        <div className="flex flex-col gap-3 border-b bg-gradient-to-r from-slate-950 via-slate-900 to-orange-950 p-4 text-white sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="flex items-center gap-2">
              <Badge variant="secondary">{platformLabel}</Badge>
              <Badge variant="outline" className="border-white/30 text-white">
                {statusText[status]}
              </Badge>
            </div>
            <p className="mt-2 text-sm text-white/70">
              在远程浏览器中完成官方登录，完成后点击“我已登录”。
            </p>
          </div>
          <div className="flex items-center gap-2">
            <div className="rounded-full border border-white/20 px-3 py-1 text-sm text-white/80">
              剩余 {formatCountdown(expiresAt)}
            </div>
            <Button type="button" variant="secondary" onClick={onCancel}>
              <X className="size-4" />
              取消
            </Button>
          </div>
        </div>

        <div className="min-h-0 flex-1 bg-slate-950 p-3">
          {streamURL ? (
            <iframe
              title={`${platformLabel} remote browser`}
              src={streamURL}
              className="h-full w-full rounded-xl border border-white/10 bg-white"
              allow="clipboard-write"
            />
          ) : (
            <div className="flex h-full items-center justify-center rounded-xl border border-white/10 text-white">
              <Loader2 className="mr-2 size-5 animate-spin" />
              正在启动远程浏览器
            </div>
          )}
        </div>

        <div className="flex flex-col gap-3 border-t p-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-h-6 text-sm">
            {error ? (
              <span className="inline-flex items-center gap-2 text-destructive">
                <AlertTriangle className="size-4" />
                {error}
              </span>
            ) : status === "connected" ? (
              <span className="inline-flex items-center gap-2 text-emerald-600">
                <CheckCircle2 className="size-4" />
                Cookie 已保存，账号可用于发布。
              </span>
            ) : (
              <span className="text-muted-foreground">
                页面内不会显示或返回原始 VNC/CDP 地址。
              </span>
            )}
          </div>
          <Button
            type="button"
            onClick={onComplete}
            disabled={!canComplete || completing}
            className="w-full sm:w-auto"
          >
            {completing ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <CheckCircle2 className="size-4" />
            )}
            我已登录
          </Button>
        </div>
      </div>
    </div>
  );
}

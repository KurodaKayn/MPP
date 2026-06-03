"use client";

import { AlertTriangle, CheckCircle2, Loader2, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { type BrowserSessionStatus } from "@/lib/dashboard/api";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";

type RemoteBrowserSessionModalProps = {
  completing: boolean;
  completeLabel?: string;
  error?: string;
  expiresAt?: string;
  platformLabel: string;
  status: BrowserSessionStatus;
  streamURL?: string;
  onCancel: () => void;
  onComplete: () => void;
  onStreamError?: () => void;
};

export function RemoteBrowserSessionModal({
  completing,
  completeLabel,
  error,
  expiresAt,
  platformLabel,
  status,
  streamURL,
  onCancel,
  onComplete,
  onStreamError,
}: RemoteBrowserSessionModalProps) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");

  const canComplete =
    status === "ready" || status === "login_detected" || status === "capturing";

  const formatCountdown = (value?: string) => {
    if (!value) {
      return t("auth.remoteBrowser.countdown.default");
    }

    const ms = new Date(value).valueOf() - Date.now();
    if (!Number.isFinite(ms) || ms <= 0) {
      return t("auth.remoteBrowser.countdown.expiring");
    }

    const minutes = Math.floor(ms / 60000);
    const seconds = Math.floor((ms % 60000) / 1000);
    return `${minutes}:${String(seconds).padStart(2, "0")}`;
  };

  return (
    <div className="fixed inset-0 z-50 bg-background/85 p-3 backdrop-blur-sm sm:p-6">
      <div className="mx-auto flex h-full max-w-7xl flex-col overflow-hidden rounded-2xl border bg-background shadow-2xl">
        <div className="flex flex-col gap-3 border-b bg-gradient-to-r from-slate-950 via-slate-900 to-orange-950 p-4 text-white sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="flex items-center gap-2">
              <Badge variant="secondary">{platformLabel}</Badge>
              <Badge variant="outline" className="border-white/30 text-white">
                {t(`auth.remoteBrowser.status.${status}`)}
              </Badge>
            </div>
            <p className="mt-2 text-sm text-white/70">
              {t("auth.remoteBrowser.description")}
            </p>
          </div>
          <div className="flex items-center gap-2">
            <div className="rounded-full border border-white/20 px-3 py-1 text-sm text-white/80">
              {t("auth.remoteBrowser.countdown.remaining", {
                time: formatCountdown(expiresAt),
              })}
            </div>
            <Button type="button" variant="secondary" onClick={onCancel}>
              <X className="size-4" />
              {t("auth.actions.cancel")}
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
              onError={onStreamError}
            />
          ) : (
            <div className="flex h-full items-center justify-center rounded-xl border border-white/10 text-white">
              <Loader2 className="mr-2 size-5 animate-spin" />
              {t("auth.remoteBrowser.starting")}
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
                {t("auth.remoteBrowser.success")}
              </span>
            ) : (
              <span className="text-muted-foreground">
                {t("auth.remoteBrowser.privacyHint")}
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
            {completeLabel ?? t("auth.remoteBrowser.complete")}
          </Button>
        </div>
      </div>
    </div>
  );
}

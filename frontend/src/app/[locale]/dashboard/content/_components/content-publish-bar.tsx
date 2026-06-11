"use client";

import { Button } from "@/components/ui/button";
import {
  AUTO_PUBLISH_PLATFORM_TABS,
  type PlatformTab,
} from "@/lib/content/platforms";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { ScheduledPublication } from "@/lib/dashboard/api";
import { cn } from "@/lib/utils";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import { CalendarClock, Loader2, RotateCcw, Send, XCircle } from "lucide-react";
import Image from "next/image";
import { useMemo, useState } from "react";

type PublishPlatform = PlatformTab["value"];

type ContentPublishBarProps = {
  canOpenXPostIntent: boolean;
  canPublish: boolean;
  canSelectPlatforms: boolean;
  isOpeningXPostIntent: boolean;
  isPublishing: boolean;
  isSchedulingPublication?: boolean;
  busyScheduleId?: string;
  onOpenDouyinPublishSession: () => void;
  onOpenXPostIntent: () => void;
  onPublish: () => void;
  onCancelSchedule?: (scheduleId: string) => void;
  onRetrySchedule?: (scheduleId: string) => void;
  onSchedulePublication?: (
    platform: PublishPlatform,
    scheduledAt: string,
  ) => void;
  onSelectedPlatformsChange: (platforms: PublishPlatform[]) => void;
  publishLabel?: string;
  scheduledPublications?: ScheduledPublication[];
  selectedPlatforms: PublishPlatform[];
};

export function ContentPublishBar({
  canOpenXPostIntent,
  canPublish,
  canSelectPlatforms,
  busyScheduleId = "",
  isOpeningXPostIntent,
  isPublishing,
  isSchedulingPublication = false,
  onCancelSchedule,
  onOpenDouyinPublishSession,
  onOpenXPostIntent,
  onPublish,
  onRetrySchedule,
  onSchedulePublication,
  onSelectedPlatformsChange,
  publishLabel,
  scheduledPublications = [],
  selectedPlatforms,
}: ContentPublishBarProps) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");
  const [schedulePlatform, setSchedulePlatform] =
    useState<PublishPlatform>("wechat");
  const [scheduledAt, setScheduledAt] = useState("");
  const isBusy = isOpeningXPostIntent || isPublishing || isSchedulingPublication;
  const selectedSet = new Set(selectedPlatforms);
  const scheduleablePlatforms = useMemo(
    () =>
      AUTO_PUBLISH_PLATFORM_TABS.filter((platform) =>
        selectedSet.has(platform.value),
      ),
    [selectedPlatforms],
  );
  const activeSchedulePlatform = scheduleablePlatforms.some(
    (platform) => platform.value === schedulePlatform,
  )
    ? schedulePlatform
    : scheduleablePlatforms[0]?.value;

  const togglePlatform = (platform: PublishPlatform, checked: boolean) => {
    if (!canSelectPlatforms) {
      return;
    }

    if (checked) {
      onSelectedPlatformsChange([...selectedPlatforms, platform]);
      return;
    }

    onSelectedPlatformsChange(
      selectedPlatforms.filter((item) => item !== platform),
    );
  };

  const formatScheduleDate = (value: string) =>
    new Intl.DateTimeFormat(locale, {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(value));

  const statusLabel = (status: string) =>
    t(`publish.scheduleStatus.${status}`, { defaultValue: status });

  return (
    <section
      aria-labelledby="publish-platforms-title"
      className="sticky bottom-4 z-20 rounded-lg border bg-background/95 p-4 shadow-sm backdrop-blur supports-[backdrop-filter]:bg-background/80"
    >
      <div className="grid gap-5">
        <div>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0">
              <h3
                id="publish-platforms-title"
                className="text-sm font-semibold"
              >
                {t("publish.autoTitle")}
              </h3>
              <p className="mt-1 text-xs text-muted-foreground">
                {t("publish.autoDesc")}
              </p>
            </div>
            <Button
              type="button"
              size="lg"
              onClick={onPublish}
              disabled={!canPublish || isBusy}
              className="h-9 w-full shrink-0 justify-center sm:w-48"
            >
              {isPublishing ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Send className="h-4 w-4" />
              )}
              {publishLabel || t("publish.buttonLabel")}
            </Button>
          </div>

          <TooltipProvider>
            <div className="mt-4 grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
              {AUTO_PUBLISH_PLATFORM_TABS.map((platform) => {
                const checked = selectedSet.has(platform.value);
                const card = (
                  <label
                    key={platform.value}
                    className={cn(
                      "flex h-14 items-center gap-3 rounded-lg border px-3 text-sm transition-colors",
                      canSelectPlatforms
                        ? "cursor-pointer"
                        : "cursor-not-allowed opacity-60",
                      checked
                        ? "border-primary bg-primary/5 text-foreground"
                        : "border-border bg-background hover:bg-muted/50",
                    )}
                  >
                    <input
                      type="checkbox"
                      checked={checked}
                      disabled={!canSelectPlatforms}
                      className="size-4 rounded border-input accent-primary"
                      onChange={(event) =>
                        togglePlatform(
                          platform.value,
                          event.currentTarget.checked,
                        )
                      }
                    />
                    <Image
                      src={platform.icon}
                      alt=""
                      width={18}
                      height={18}
                      aria-hidden="true"
                      className="size-[18px] shrink-0"
                    />
                    <span className="truncate font-medium">
                      {t(platform.label, {
                        defaultValue: platform.defaultLabel,
                      })}
                    </span>
                  </label>
                );

                if (canSelectPlatforms) {
                  return card;
                }

                return (
                  <Tooltip key={platform.value}>
                    <TooltipTrigger render={<div />}>{card}</TooltipTrigger>
                    <TooltipContent>
                      {t("publish.selectPlatformHint")}
                    </TooltipContent>
                  </Tooltip>
                );
              })}
            </div>
          </TooltipProvider>
        </div>

        <div className="border-t pt-4">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
            <div className="min-w-0">
              <h3 className="text-sm font-semibold">
                {t("publish.scheduleTitle", { defaultValue: "Scheduled publish" })}
              </h3>
              <p className="mt-1 text-xs text-muted-foreground">
                {t("publish.scheduleDesc", {
                  defaultValue:
                    "Schedule synced platform drafts and manage failed attempts.",
                })}
              </p>
            </div>
            <div className="grid gap-2 sm:grid-cols-[minmax(120px,160px)_minmax(180px,220px)_auto]">
              <select
                value={activeSchedulePlatform ?? ""}
                disabled={!canPublish || isBusy || !scheduleablePlatforms.length}
                onChange={(event) =>
                  setSchedulePlatform(event.target.value as PublishPlatform)
                }
                className="h-9 rounded-md border bg-background px-3 text-sm outline-none ring-offset-background focus-visible:ring-2 focus-visible:ring-ring"
              >
                {scheduleablePlatforms.map((platform) => (
                  <option key={platform.value} value={platform.value}>
                    {t(platform.label, {
                      defaultValue: platform.defaultLabel,
                    })}
                  </option>
                ))}
              </select>
              <input
                type="datetime-local"
                value={scheduledAt}
                disabled={!canPublish || isBusy}
                onChange={(event) => setScheduledAt(event.currentTarget.value)}
                className="h-9 rounded-md border bg-background px-3 text-sm outline-none ring-offset-background focus-visible:ring-2 focus-visible:ring-ring"
              />
              <Button
                type="button"
                size="sm"
                variant="secondary"
                disabled={
                  !canPublish ||
                  isBusy ||
                  !activeSchedulePlatform ||
                  !scheduledAt
                }
                onClick={() => {
                  if (activeSchedulePlatform) {
                    onSchedulePublication?.(activeSchedulePlatform, scheduledAt);
                  }
                }}
                className="h-9"
              >
                {isSchedulingPublication ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <CalendarClock className="h-4 w-4" />
                )}
                {t("publish.scheduleButton", { defaultValue: "Schedule" })}
              </Button>
            </div>
          </div>

          {scheduledPublications.length ? (
            <div className="mt-3 grid gap-2">
              {scheduledPublications.map((schedule) => {
                const isScheduleBusy = busyScheduleId === schedule.id;
                const lastAttempt =
                  schedule.attempts[schedule.attempts.length - 1];
                return (
                  <div
                    key={schedule.id}
                    className="flex flex-col gap-2 rounded-lg border bg-muted/20 px-3 py-2 text-xs sm:flex-row sm:items-center sm:justify-between"
                  >
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-medium">
                          {schedule.platform}
                        </span>
                        <span className="text-muted-foreground">
                          {formatScheduleDate(schedule.scheduled_at)}
                        </span>
                        <span className="rounded-md bg-background px-2 py-0.5 text-muted-foreground">
                          {statusLabel(schedule.status)}
                        </span>
                      </div>
                      {schedule.last_error || lastAttempt?.error_message ? (
                        <p className="mt-1 truncate text-destructive">
                          {schedule.last_error || lastAttempt?.error_message}
                        </p>
                      ) : null}
                      {schedule.manual_action_url ? (
                        <a
                          href={schedule.manual_action_url}
                          target="_blank"
                          rel="noreferrer"
                          className="mt-1 inline-flex text-primary underline-offset-4 hover:underline"
                        >
                          {t("publish.manualAction", {
                            defaultValue: "Open manual action",
                          })}
                        </a>
                      ) : null}
                    </div>
                    <div className="flex shrink-0 gap-2">
                      {schedule.status === "failed" ||
                      schedule.status === "needs_manual_action" ? (
                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          disabled={isBusy || isScheduleBusy}
                          onClick={() => onRetrySchedule?.(schedule.id)}
                          className="h-8"
                        >
                          {isScheduleBusy ? (
                            <Loader2 className="h-3.5 w-3.5 animate-spin" />
                          ) : (
                            <RotateCcw className="h-3.5 w-3.5" />
                          )}
                          {t("common.retry", { defaultValue: "Retry" })}
                        </Button>
                      ) : null}
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={
                          isBusy ||
                          isScheduleBusy ||
                          schedule.status === "running" ||
                          schedule.status === "published" ||
                          schedule.status === "cancelled"
                        }
                        onClick={() => onCancelSchedule?.(schedule.id)}
                        className="h-8"
                      >
                        {isScheduleBusy ? (
                          <Loader2 className="h-3.5 w-3.5 animate-spin" />
                        ) : (
                          <XCircle className="h-3.5 w-3.5" />
                        )}
                        {t("common.cancel", { defaultValue: "Cancel" })}
                      </Button>
                    </div>
                  </div>
                );
              })}
            </div>
          ) : null}
        </div>

        <div className="border-t pt-4">
          <h3 className="text-sm font-semibold">{t("publish.manualTitle")}</h3>
          <div className="mt-4 grid gap-2 sm:grid-cols-2 xl:grid-cols-5">
            <Button
              type="button"
              size="lg"
              variant="outline"
              onClick={onOpenXPostIntent}
              disabled={!canOpenXPostIntent || isBusy}
              className="h-14 justify-start gap-3 rounded-lg px-3 text-sm font-medium"
            >
              {isOpeningXPostIntent ? (
                <Loader2 className="size-[18px] animate-spin" />
              ) : (
                <Image
                  src="/icons/platforms/x.svg"
                  alt=""
                  width={18}
                  height={18}
                  aria-hidden="true"
                  className="size-[18px]"
                />
              )}
              <span className="truncate">X</span>
            </Button>
            <Button
              type="button"
              size="lg"
              variant="outline"
              onClick={onOpenDouyinPublishSession}
              disabled={!canOpenXPostIntent || isBusy}
              className="h-14 justify-start gap-3 rounded-lg px-3 text-sm font-medium"
            >
              {isOpeningXPostIntent ? (
                <Loader2 className="size-[18px] animate-spin" />
              ) : (
                <Image
                  src="/icons/platforms/douyin.svg"
                  alt=""
                  width={18}
                  height={18}
                  aria-hidden="true"
                  className="size-[18px]"
                />
              )}
              <span className="truncate">
                {t("platforms.douyin", { defaultValue: "Douyin" })}
              </span>
            </Button>
          </div>
        </div>
      </div>
    </section>
  );
}

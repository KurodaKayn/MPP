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
import { cn } from "@/lib/utils";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import { Loader2, Send } from "lucide-react";
import Image from "next/image";

type PublishPlatform = PlatformTab["value"];

type ContentPublishBarProps = {
  canOpenXPostIntent: boolean;
  canPublish: boolean;
  canSelectPlatforms: boolean;
  isOpeningXPostIntent: boolean;
  isPublishing: boolean;
  onOpenDouyinPublishSession: () => void;
  onOpenXPostIntent: () => void;
  onPublish: () => void;
  onSelectedPlatformsChange: (platforms: PublishPlatform[]) => void;
  publishLabel?: string;
  selectedPlatforms: PublishPlatform[];
};

export function ContentPublishBar({
  canOpenXPostIntent,
  canPublish,
  canSelectPlatforms,
  isOpeningXPostIntent,
  isPublishing,
  onOpenDouyinPublishSession,
  onOpenXPostIntent,
  onPublish,
  onSelectedPlatformsChange,
  publishLabel,
  selectedPlatforms,
}: ContentPublishBarProps) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");
  const isBusy = isOpeningXPostIntent || isPublishing;
  const selectedSet = new Set(selectedPlatforms);

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

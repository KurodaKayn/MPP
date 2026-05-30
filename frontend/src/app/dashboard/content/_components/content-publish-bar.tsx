import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { PLATFORM_TABS, type PlatformTab } from "@/lib/content/platforms";
import { Check, Loader2, Send } from "lucide-react";
import Image from "next/image";

type PublishPlatform = PlatformTab["value"];

type ContentPublishBarProps = {
  canOpenXPostIntent: boolean;
  canPublish: boolean;
  isOpeningXPostIntent: boolean;
  isPublishing: boolean;
  onOpenXPostIntent: () => void;
  onPublish: () => void;
  onSelectedPlatformsChange: (platforms: PublishPlatform[]) => void;
  publishLabel?: string;
  selectedPlatforms: PublishPlatform[];
};

export function ContentPublishBar({
  canOpenXPostIntent,
  canPublish,
  isOpeningXPostIntent,
  isPublishing,
  onOpenXPostIntent,
  onPublish,
  onSelectedPlatformsChange,
  publishLabel = "一键发布",
  selectedPlatforms,
}: ContentPublishBarProps) {
  const selectedSet = new Set(selectedPlatforms);
  const allSelected = selectedPlatforms.length === PLATFORM_TABS.length;
  const isBusy = isOpeningXPostIntent || isPublishing;

  const togglePlatform = (platform: PublishPlatform, checked: boolean) => {
    if (checked) {
      onSelectedPlatformsChange([...selectedPlatforms, platform]);
      return;
    }

    onSelectedPlatformsChange(
      selectedPlatforms.filter((item) => item !== platform),
    );
  };

  const toggleAll = (checked: boolean) => {
    onSelectedPlatformsChange(
      checked ? PLATFORM_TABS.map((platform) => platform.value) : [],
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
                自动发布
              </h3>
              <label className="mt-2 inline-flex w-fit cursor-pointer items-center gap-2 text-xs text-muted-foreground">
                <input
                  type="checkbox"
                  checked={allSelected}
                  onChange={(event) => toggleAll(event.currentTarget.checked)}
                  className="size-4 rounded border-input accent-primary"
                />
                全选平台
              </label>
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
              {publishLabel}
            </Button>
          </div>

          <div className="mt-4 grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
            {PLATFORM_TABS.map((platform) => {
              const checked = selectedSet.has(platform.value);

              return (
                <label
                  key={platform.value}
                  className={cn(
                    "flex h-14 cursor-pointer items-center gap-3 rounded-lg border px-3 text-sm transition-colors",
                    checked
                      ? "border-primary bg-primary/5 text-foreground"
                      : "border-border bg-background hover:bg-muted/50",
                  )}
                >
                  <input
                    type="checkbox"
                    checked={checked}
                    className="sr-only"
                    onChange={(event) =>
                      togglePlatform(
                        platform.value,
                        event.currentTarget.checked,
                      )
                    }
                  />
                  <span
                    aria-hidden="true"
                    className={cn(
                      "flex size-4 shrink-0 items-center justify-center rounded-sm border",
                      checked
                        ? "border-primary bg-primary text-primary-foreground"
                        : "border-input bg-background",
                    )}
                  >
                    {checked ? <Check className="size-3" /> : null}
                  </span>
                  <Image
                    src={platform.icon}
                    alt=""
                    width={18}
                    height={18}
                    aria-hidden="true"
                    className="size-[18px] shrink-0"
                  />
                  <span className="truncate font-medium">{platform.label}</span>
                </label>
              );
            })}
          </div>
        </div>

        <div className="border-t pt-4">
          <h3 className="text-sm font-semibold">手动发布</h3>
          <div className="mt-4 grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
            <Button
              type="button"
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
          </div>
        </div>
      </div>
    </section>
  );
}

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { PLATFORM_TABS, type PlatformTab } from "@/lib/content/platforms";
import { Check, Send } from "lucide-react";
import Image from "next/image";

type PublishPlatform = PlatformTab["value"];

type ContentPublishBarProps = {
  canPublish: boolean;
  onPublish: () => void;
  onSelectedPlatformsChange: (platforms: PublishPlatform[]) => void;
  selectedPlatforms: PublishPlatform[];
};

export function ContentPublishBar({
  canPublish,
  onPublish,
  onSelectedPlatformsChange,
  selectedPlatforms,
}: ContentPublishBarProps) {
  const selectedSet = new Set(selectedPlatforms);
  const allSelected = selectedPlatforms.length === PLATFORM_TABS.length;

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
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 id="publish-platforms-title" className="text-sm font-semibold">
            发布渠道
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
          onClick={onPublish}
          disabled={!canPublish}
          className="shrink-0"
        >
          <Send className="h-4 w-4" /> 一键发布
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
                  togglePlatform(platform.value, event.currentTarget.checked)
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
    </section>
  );
}

import { Button } from "@/components/ui/button";
import { PLATFORM_TABS, type PlatformTab } from "@/lib/content/platforms";
import { Loader2, Send } from "lucide-react";
import Image from "next/image";

type PublishPlatform = PlatformTab["value"];

type ContentPublishBarProps = {
  canOpenXPostIntent: boolean;
  canPublish: boolean;
  isOpeningXPostIntent: boolean;
  isPublishing: boolean;
  onOpenXPostIntent: () => void;
  onPublish: () => void;
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
  publishLabel = "一键发布",
  selectedPlatforms,
}: ContentPublishBarProps) {
  const isBusy = isOpeningXPostIntent || isPublishing;
  const selectedTabs = PLATFORM_TABS.filter((platform) =>
    selectedPlatforms.includes(platform.value),
  );

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
              <p className="mt-1 text-xs text-muted-foreground">
                使用预发布区块中已选择的平台。
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
              {publishLabel}
            </Button>
          </div>

          <div className="mt-4 flex flex-wrap gap-2">
            {selectedTabs.length > 0 ? (
              selectedTabs.map((platform) => (
                <div
                  key={platform.value}
                  className="flex h-8 items-center gap-2 rounded-md border bg-muted/30 px-2 text-xs font-medium"
                >
                  <Image
                    src={platform.icon}
                    alt=""
                    width={16}
                    height={16}
                    aria-hidden="true"
                    className="size-4 shrink-0"
                  />
                  <span>{platform.label}</span>
                </div>
              ))
            ) : (
              <div className="text-sm text-muted-foreground">
                尚未选择发布平台。
              </div>
            )}
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

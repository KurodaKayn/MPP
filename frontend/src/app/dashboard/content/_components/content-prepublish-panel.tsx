"use client";

import { Check, Loader2, RefreshCw } from "lucide-react";
import Image from "next/image";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { PLATFORM_TABS, type PlatformTab } from "@/lib/content/platforms";
import type { ContentValue } from "@/lib/content/types";
import { cn } from "@/lib/utils";

type PublishPlatform = PlatformTab["value"];
type PrepublishFormat = "html" | "markdown" | "text";

export type PrepublishDraft = {
  format: PrepublishFormat;
  raw: string;
  syncedAt: string;
};

type ContentPrepublishPanelProps = {
  content: ContentValue;
  drafts: Partial<Record<PublishPlatform, PrepublishDraft>>;
  isSyncing: boolean;
  onSelectedPlatformsChange: (platforms: PublishPlatform[]) => void;
  onSync: () => void;
  selectedPlatforms: PublishPlatform[];
  title: string;
};

const platformFormats: Record<PublishPlatform, PrepublishFormat> = {
  bilibili: "text",
  wechat: "html",
  x: "text",
  xiaohongshu: "text",
  zhihu: "markdown",
};

function formatLabel(format: PrepublishFormat) {
  switch (format) {
    case "html":
      return "HTML";
    case "markdown":
      return "Markdown";
    case "text":
      return "Text";
  }
}

function platformLabel(platform: PublishPlatform) {
  return (
    PLATFORM_TABS.find((item) => item.value === platform)?.label ?? platform
  );
}

function renderPreview(draft: PrepublishDraft, title: string) {
  if (draft.format === "html") {
    return (
      <article className="prose prose-sm max-w-none dark:prose-invert">
        {title ? <h1>{title}</h1> : null}
        <div dangerouslySetInnerHTML={{ __html: draft.raw }} />
      </article>
    );
  }

  return (
    <article className="space-y-4 whitespace-pre-wrap text-sm leading-6">
      {title ? <h2 className="text-lg font-semibold">{title}</h2> : null}
      <div>{draft.raw}</div>
    </article>
  );
}

export function ContentPrepublishPanel({
  content,
  drafts,
  isSyncing,
  onSelectedPlatformsChange,
  onSync,
  selectedPlatforms,
  title,
}: ContentPrepublishPanelProps) {
  const selectedSet = new Set(selectedPlatforms);
  const allSelected = selectedPlatforms.length === PLATFORM_TABS.length;
  const activePlatform = selectedPlatforms[0] ?? PLATFORM_TABS[0].value;
  const hasSourceContent = Boolean(
    content.text.trim() || content.firstImageSrc,
  );

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
    <Card>
      <CardHeader className="gap-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle>预发布</CardTitle>
            <p className="mt-1 text-sm text-muted-foreground">
              同步后查看各平台将使用的原始格式和预览。
            </p>
          </div>
          <Button
            type="button"
            onClick={onSync}
            disabled={!hasSourceContent || selectedPlatforms.length === 0}
            className="w-full sm:w-auto"
          >
            {isSyncing ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <RefreshCw className="size-4" />
            )}
            同步到预发布
          </Button>
        </div>

        <label className="inline-flex w-fit cursor-pointer items-center gap-2 text-xs text-muted-foreground">
          <input
            type="checkbox"
            checked={allSelected}
            onChange={(event) => toggleAll(event.currentTarget.checked)}
            className="size-4 rounded border-input accent-primary"
          />
          全选平台
        </label>
      </CardHeader>

      <CardContent className="space-y-5">
        <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-5">
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

        {selectedPlatforms.length === 0 ? (
          <div className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">
            选择平台后同步预发布内容。
          </div>
        ) : (
          <Tabs defaultValue={activePlatform} className="w-full">
            <TabsList
              className="grid w-full"
              style={{
                gridTemplateColumns: `repeat(${selectedPlatforms.length}, minmax(0, 1fr))`,
              }}
            >
              {selectedPlatforms.map((platform) => (
                <TabsTrigger key={platform} value={platform}>
                  {platformLabel(platform)}
                </TabsTrigger>
              ))}
            </TabsList>

            {selectedPlatforms.map((platform) => {
              const draft = drafts[platform];
              const expectedFormat = platformFormats[platform];

              return (
                <TabsContent key={platform} value={platform} className="mt-4">
                  {draft ? (
                    <Tabs defaultValue="raw" className="w-full">
                      <div className="mb-3 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                        <div className="text-sm text-muted-foreground">
                          {formatLabel(draft.format)} · 已同步{" "}
                          {new Date(draft.syncedAt).toLocaleString()}
                        </div>
                        <TabsList>
                          <TabsTrigger value="raw">原始格式</TabsTrigger>
                          <TabsTrigger value="preview">预览</TabsTrigger>
                        </TabsList>
                      </div>
                      <TabsContent value="raw" className="mt-0">
                        <ScrollArea className="h-80 rounded-lg border bg-muted/30">
                          <pre className="p-4 text-xs leading-5 whitespace-pre-wrap">
                            {draft.raw}
                          </pre>
                        </ScrollArea>
                      </TabsContent>
                      <TabsContent value="preview" className="mt-0">
                        <ScrollArea className="h-80 rounded-lg border p-4">
                          {renderPreview(draft, title)}
                        </ScrollArea>
                      </TabsContent>
                    </Tabs>
                  ) : (
                    <div className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">
                      尚未同步。当前平台目标格式：
                      {formatLabel(expectedFormat)}。
                    </div>
                  )}
                </TabsContent>
              );
            })}
          </Tabs>
        )}
      </CardContent>
    </Card>
  );
}

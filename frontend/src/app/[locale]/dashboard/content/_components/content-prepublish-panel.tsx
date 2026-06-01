"use client";

import { Loader2, RefreshCw } from "lucide-react";
import Image from "next/image";
import { useState } from "react";
import { toast } from "sonner";
import { AIEditAssistant } from "@/components/dashboard/content/ai/ai-edit-assistant";
import { AIMarkdownPreview } from "@/components/dashboard/content/ai/ai-markdown-preview";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ScrollArea } from "@/components/ui/scroll-area";
import { PLATFORM_TABS, type PlatformTab } from "@/lib/content/platforms";
import type { ContentValue } from "@/lib/content/types";
import {
  streamAIPrepublishEdit,
  updateProjectPrepublishDraft,
  type AdaptedContent,
} from "@/lib/dashboard/api";
import { cn } from "@/lib/utils";
import type {
  PrepublishDraft,
  PrepublishFormat,
} from "../_stores/content-page-store";

type PublishPlatform = PlatformTab["value"];

type ContentPrepublishPanelProps = {
  content: ContentValue;
  drafts: Partial<Record<PublishPlatform, PrepublishDraft>>;
  isSyncing: boolean;
  onSync: (platforms?: PublishPlatform[]) => void;
  onDraftChange: (platform: PublishPlatform, draft: PrepublishDraft) => void;
  projectId?: string;
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
    PLATFORM_TABS.find((item) => item.value === platform)?.defaultLabel ??
    platform
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
      {draft.format === "markdown" ? (
        <AIMarkdownPreview markdown={draft.raw} />
      ) : (
        <div>{draft.raw}</div>
      )}
    </article>
  );
}

export function ContentPrepublishPanel({
  content,
  drafts,
  isSyncing,
  onDraftChange,
  onSync,
  projectId,
  title,
}: ContentPrepublishPanelProps) {
  const hasSourceContent = Boolean(
    content.text.trim() || content.firstImageSrc,
  );
  const [activePlatform, setActivePlatform] = useState<PublishPlatform>(
    PLATFORM_TABS[0].value,
  );

  const activatePlatform = (platform: PublishPlatform) => {
    setActivePlatform(platform);
  };

  const activeDraft = drafts[activePlatform];
  const expectedFormat = platformFormats[activePlatform];
  const canUseAI = Boolean(projectId && activeDraft);

  return (
    <Card>
      <CardHeader className="gap-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle>预发布</CardTitle>
            <p className="mt-1 text-sm text-muted-foreground">
              点击平台进入专属预发布区块，左侧查看原始格式，右侧查看预览。
            </p>
          </div>
          <div className="flex w-full flex-col gap-2 sm:w-auto sm:flex-row">
            <Button
              type="button"
              size="lg"
              variant="outline"
              onClick={() => onSync([activePlatform])}
              disabled={!hasSourceContent}
              className="h-9 w-full justify-center sm:w-48"
            >
              {isSyncing ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <RefreshCw className="size-4" />
              )}
              同步到该平台
            </Button>
            <Button
              type="button"
              size="lg"
              onClick={() =>
                onSync(PLATFORM_TABS.map((platform) => platform.value))
              }
              disabled={!hasSourceContent}
              className="h-9 w-full justify-center sm:w-48"
            >
              {isSyncing ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <RefreshCw className="size-4" />
              )}
              一键同步到所有平台
            </Button>
          </div>
        </div>
      </CardHeader>

      <CardContent className="space-y-5">
        <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-5">
          {PLATFORM_TABS.map((platform) => {
            const isActive = activePlatform === platform.value;

            return (
              <button
                type="button"
                key={platform.value}
                onClick={() => activatePlatform(platform.value)}
                className={cn(
                  "flex h-14 items-center gap-3 rounded-lg border px-3 text-left text-sm transition-colors",
                  isActive
                    ? "border-foreground/50 bg-muted text-foreground shadow-sm"
                    : "border-border bg-background hover:bg-muted/50",
                )}
              >
                <Image
                  src={platform.icon}
                  alt=""
                  width={18}
                  height={18}
                  aria-hidden="true"
                  className="size-[18px] shrink-0"
                />
                <div className="min-w-0 flex-1">
                  <div className="truncate font-medium">{platform.label}</div>
                  <div className="mt-0.5 text-[11px] text-muted-foreground">
                    {drafts[platform.value] ? "已同步" : "未同步"}
                  </div>
                </div>
              </button>
            );
          })}
        </div>

        {!hasSourceContent ? (
          <div className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">
            请先填写内容，再同步平台草稿。
          </div>
        ) : (
          <section className="space-y-3">
            <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <h3 className="text-sm font-semibold">
                  {platformLabel(activePlatform)}
                </h3>
                <p className="text-sm text-muted-foreground">
                  {activeDraft
                    ? `${formatLabel(activeDraft.format)} · 已同步 ${new Date(
                        activeDraft.syncedAt,
                      ).toLocaleString()}`
                    : `尚未同步。目标格式：${formatLabel(expectedFormat)}。`}
                </p>
              </div>
            </div>

            <AIEditAssistant
              title="AI 编辑预发布"
              source={activeDraft?.raw ?? ""}
              format={activeDraft?.format ?? expectedFormat}
              disabled={!canUseAI}
              onGenerate={(message, onChunk, signal) => {
                if (!activeDraft) {
                  throw new Error("请先同步该平台草稿");
                }
                return streamAIPrepublishEdit(
                  {
                    adapted_content: adaptedContentFromDraft(activeDraft),
                    message,
                    platform: activePlatform,
                    title,
                  },
                  {
                    onChunk,
                    signal,
                  },
                );
              }}
              onApply={async (proposal) => {
                if (!projectId || !activeDraft) {
                  throw new Error("请先同步该平台草稿");
                }
                await updateProjectPrepublishDraft(projectId, activePlatform, {
                  adapted_content: adaptedContentFromDraft(
                    activeDraft,
                    proposal,
                  ),
                });
                onDraftChange(activePlatform, {
                  ...activeDraft,
                  raw: proposal,
                  syncedAt: new Date().toISOString(),
                });
                toast.success("AI 修改已保存到预发布");
              }}
            />

            <div className="grid gap-4 xl:grid-cols-2">
              <section className="space-y-2">
                <h4 className="text-xs font-semibold uppercase tracking-[0.12em] text-muted-foreground">
                  原始格式
                </h4>
                <ScrollArea className="h-96 rounded-lg border bg-muted/30">
                  <pre className="p-4 text-xs leading-5 whitespace-pre-wrap">
                    {activeDraft?.raw ??
                      `当前平台目标格式：${formatLabel(expectedFormat)}。\n点击上方按钮同步后，这里会显示真正保存到数据库中的原始格式内容。`}
                  </pre>
                </ScrollArea>
              </section>

              <section className="space-y-2">
                <h4 className="text-xs font-semibold uppercase tracking-[0.12em] text-muted-foreground">
                  预览
                </h4>
                <ScrollArea className="h-96 rounded-lg border p-4">
                  {activeDraft ? (
                    renderPreview(activeDraft, title)
                  ) : (
                    <div className="text-sm leading-6 text-muted-foreground">
                      当前平台还没有同步草稿，预览将在同步完成后显示。
                    </div>
                  )}
                </ScrollArea>
              </section>
            </div>
          </section>
        )}
      </CardContent>
    </Card>
  );
}

function adaptedContentFromDraft(
  draft: PrepublishDraft,
  raw = draft.raw,
): AdaptedContent {
  return {
    format: draft.format,
    [draft.format]: raw,
    source_revision: new Date().toISOString(),
    generated_by: {
      type: "ai-editor",
    },
  };
}

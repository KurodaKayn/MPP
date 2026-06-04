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
  type ProjectPublications,
} from "@/lib/dashboard/api";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
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
  douyin: "text",
  wechat: "html",
  x: "text",
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

function isPrepublishFormat(
  format: AdaptedContent["format"],
): format is PrepublishFormat {
  return format === "html" || format === "markdown" || format === "text";
}

function adaptedContentFromDraft(
  draft: PrepublishDraft,
  raw = draft.raw,
): AdaptedContent {
  switch (draft.format) {
    case "html":
      return {
        format: draft.format,
        html: raw,
      };
    case "markdown":
      return {
        format: draft.format,
        markdown: raw,
      };
    case "text":
      return {
        format: draft.format,
        text: raw,
      };
  }
}

function draftFromPublications(
  publications: ProjectPublications,
  platform: PublishPlatform,
  fallback: PrepublishDraft,
): PrepublishDraft {
  const publication = publications.items.find(
    (item) => item.platform === platform,
  );
  const adaptedContent = publication?.adapted_content;
  if (!publication || !adaptedContent) {
    return fallback;
  }

  const format = isPrepublishFormat(adaptedContent.format)
    ? adaptedContent.format
    : fallback.format;
  const raw =
    adaptedContent.html ??
    adaptedContent.markdown ??
    adaptedContent.text ??
    adaptedContent.summary ??
    fallback.raw;

  return {
    format,
    raw,
    syncedAt:
      adaptedContent.source_revision ??
      publication.updated_at ??
      fallback.syncedAt,
  };
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
  const locale = useAppLocale();
  const { t: tCommon } = useTranslation(locale, "common");
  const { t } = useTranslation(locale, "dashboard");

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
  const getPlatformLabel = (platform: PlatformTab) =>
    tCommon(platform.label, { defaultValue: platform.defaultLabel });

  return (
    <Card>
      <CardHeader className="gap-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle>{t("content.prepublish.title")}</CardTitle>
            <p className="mt-1 text-sm text-muted-foreground">
              {t("content.prepublish.description")}
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
              {t("content.prepublish.syncToPlatform")}
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
              {t("content.prepublish.syncAll")}
            </Button>
          </div>
        </div>

        <div className="flex flex-wrap gap-2 pt-2">
          {PLATFORM_TABS.map((platform) => (
            <button
              type="button"
              key={platform.value}
              onClick={() => activatePlatform(platform.value)}
              className={cn(
                "group relative flex min-w-[120px] flex-col rounded-lg border p-3 text-left transition-all hover:bg-muted/50",
                activePlatform === platform.value
                  ? "border-primary bg-primary/5 ring-1 ring-primary"
                  : "border-muted-foreground/10 bg-muted/20",
              )}
            >
              <div className="flex items-center gap-2">
                <Image
                  src={platform.icon}
                  alt={getPlatformLabel(platform)}
                  width={16}
                  height={16}
                  className={cn(
                    "grayscale transition-all group-hover:grayscale-0",
                    activePlatform === platform.value && "grayscale-0",
                  )}
                />
                <span className="text-sm font-medium">
                  {getPlatformLabel(platform)}
                </span>
              </div>
              <div className="mt-1 text-[10px] text-muted-foreground">
                {drafts[platform.value]
                  ? t("content.prepublish.statusSynced")
                  : t("content.prepublish.statusNotSynced")}
              </div>
            </button>
          ))}
        </div>
      </CardHeader>

      <CardContent className="grid h-[600px] gap-4 p-4 pt-0 md:grid-cols-2">
        {!hasSourceContent ? (
          <div className="col-span-2 flex h-full items-center justify-center text-sm text-muted-foreground">
            {t("content.prepublish.pleaseFillContent")}
          </div>
        ) : (
          <>
            <div className="flex flex-col gap-4 overflow-hidden">
              {canUseAI ? (
                <div className="px-1">
                  <AIEditAssistant
                    className="w-full"
                    title={t("content.prepublish.aiEditTitle")}
                    format={activeDraft?.format}
                    source={activeDraft?.raw ?? ""}
                    onGenerate={(message, onChunk, signal) => {
                      if (!projectId || !activeDraft) {
                        throw new Error(t("content.prepublish.errorSyncFirst"));
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
                    onApply={async (newContent) => {
                      if (!projectId || !activeDraft) {
                        throw new Error(t("content.prepublish.errorSyncFirst"));
                      }
                      const fallbackDraft: PrepublishDraft = {
                        ...activeDraft,
                        raw: newContent,
                        syncedAt: new Date().toISOString(),
                      };
                      const publications = await updateProjectPrepublishDraft(
                        projectId,
                        activePlatform,
                        {
                          adapted_content: adaptedContentFromDraft(
                            activeDraft,
                            newContent,
                          ),
                        },
                      );
                      onDraftChange(
                        activePlatform,
                        draftFromPublications(
                          publications,
                          activePlatform,
                          fallbackDraft,
                        ),
                      );
                      toast.success(t("content.prepublish.aiSaveSuccess"));
                    }}
                  />
                </div>
              ) : null}
              <Card className="flex-1 overflow-hidden bg-muted/30">
                <CardHeader className="py-2">
                  <CardTitle className="text-xs text-muted-foreground">
                    {t("content.prepublish.originalFormat")}
                  </CardTitle>
                </CardHeader>
                <CardContent className="h-full p-0">
                  <ScrollArea className="h-[480px] w-full">
                    {activeDraft ? (
                      <pre className="p-4 text-xs leading-5">
                        {activeDraft.raw}
                      </pre>
                    ) : (
                      <div className="flex h-full items-center justify-center p-8 text-center text-xs text-muted-foreground">
                        {t("content.prepublish.originalDesc", {
                          format: formatLabel(expectedFormat),
                        })}
                      </div>
                    )}
                  </ScrollArea>
                </CardContent>
              </Card>
            </div>

            <div className="flex flex-col gap-4 overflow-hidden">
              <div className="flex h-8 items-center px-1">
                <h3 className="text-sm font-medium">
                  {t("content.prepublish.preview")}
                </h3>
              </div>
              <Card className="flex-1 overflow-hidden border-primary/20 shadow-inner">
                <CardContent className="h-full p-0">
                  <ScrollArea className="h-[520px] w-full">
                    <div className="p-6">
                      {activeDraft ? (
                        renderPreview(activeDraft, title)
                      ) : (
                        <div className="flex h-[400px] items-center justify-center text-center text-sm text-muted-foreground">
                          {t("content.prepublish.previewPlaceholder")}
                        </div>
                      )}
                    </div>
                  </ScrollArea>
                </CardContent>
              </Card>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}

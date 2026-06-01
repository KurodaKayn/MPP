"use client";

import {
  Bot,
  Check,
  FileText,
  GitCompareArrows,
  Loader2,
  Square,
  X,
} from "lucide-react";
import { useRef, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import { AIDiffPreview } from "./ai-diff-preview";
import { AIMarkdownPreview } from "./ai-markdown-preview";

type AIProposalFormat = "html" | "markdown" | "text";

type AIEditAssistantProps = {
  className?: string;
  disabled?: boolean;
  format?: AIProposalFormat;
  onApply: (proposal: string) => void | Promise<void>;
  onGenerate: (
    message: string,
    onChunk: (chunk: string, accumulated: string) => void,
    signal: AbortSignal,
  ) => Promise<string>;
  source: string;
  title: string;
};

type AIEditStatus = "idle" | "streaming" | "ready";
type AIReviewView = "preview" | "diff";

export function AIEditAssistant({
  className,
  disabled,
  format = "markdown",
  onApply,
  onGenerate,
  source,
  title,
}: AIEditAssistantProps) {
  const abortControllerRef = useRef<AbortController | null>(null);
  const [message, setMessage] = useState("");
  const [proposal, setProposal] = useState("");
  const [snapshot, setSnapshot] = useState(source);
  const [status, setStatus] = useState<AIEditStatus>("idle");
  const [view, setView] = useState<AIReviewView>("preview");
  const [isApplying, setIsApplying] = useState(false);
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");
  const isStreaming = status === "streaming";
  const canGenerate = Boolean(
    !disabled && !isStreaming && !isApplying && message.trim(),
  );
  const hasProposal = Boolean(proposal.trim());

  const generate = async () => {
    if (!canGenerate) {
      return;
    }

    const controller = new AbortController();
    abortControllerRef.current = controller;
    const nextSnapshot = source;

    setSnapshot(nextSnapshot);
    setProposal("");
    setStatus("streaming");
    setView("preview");

    try {
      const finalProposal = await onGenerate(
        message.trim(),
        (_chunk, accumulated) => setProposal(accumulated),
        controller.signal,
      );
      setProposal(finalProposal);
      setStatus(finalProposal.trim() ? "ready" : "idle");
    } catch (error) {
      if (isAbortError(error)) {
        setStatus(proposal.trim() ? "ready" : "idle");
        return;
      }
      setStatus("idle");
      toast.error(t("ai.editFailed"), {
        description:
          error instanceof Error ? error.message : t("common.retryLater"),
      });
    } finally {
      abortControllerRef.current = null;
    }
  };

  const stop = () => {
    abortControllerRef.current?.abort();
    setStatus(proposal.trim() ? "ready" : "idle");
  };

  const accept = async () => {
    if (!proposal.trim()) {
      return;
    }
    setIsApplying(true);
    try {
      await onApply(proposal);
      setMessage("");
      setProposal("");
      setStatus("idle");
    } catch (error) {
      toast.error(t("ai.applyFailed"), {
        description:
          error instanceof Error ? error.message : t("common.retryLater"),
      });
    } finally {
      setIsApplying(false);
    }
  };

  const reject = () => {
    abortControllerRef.current?.abort();
    setProposal("");
    setStatus("idle");
  };

  return (
    <section
      className={cn(
        "rounded-lg border bg-muted/20 p-3 sm:p-4",
        "space-y-3",
        className,
      )}
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-semibold">
            <Bot className="size-4" />
            {title}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1 rounded-md border bg-background p-0.5">
          <Button
            type="button"
            size="sm"
            variant={view === "preview" ? "default" : "ghost"}
            className="h-7 px-2 text-xs"
            onClick={() => setView("preview")}
            disabled={!hasProposal}
          >
            <FileText className="size-3.5" />
            {t("common.preview")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant={view === "diff" ? "default" : "ghost"}
            className="h-7 px-2 text-xs"
            onClick={() => setView("diff")}
            disabled={!hasProposal}
          >
            <GitCompareArrows className="size-3.5" />
            {t("ai.diff")}
          </Button>
        </div>
      </div>

      <div className="grid gap-3 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
        <div className="space-y-2">
          <Textarea
            value={message}
            onChange={(event) => setMessage(event.target.value)}
            placeholder={t("ai.placeholder")}
            disabled={disabled || isStreaming}
            className="min-h-24 resize-none bg-background text-sm"
          />
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              size="sm"
              onClick={() => void generate()}
              disabled={!canGenerate}
            >
              {isStreaming ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Bot className="size-4" />
              )}
              {t("ai.generate")}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={stop}
              disabled={!isStreaming}
            >
              <Square className="size-4" />
              {t("ai.stop")}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => void accept()}
              disabled={!hasProposal || isStreaming || isApplying}
            >
              {isApplying ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Check className="size-4" />
              )}
              {t("ai.accept")}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={reject}
              disabled={!hasProposal && !isStreaming}
            >
              <X className="size-4" />
              {t("ai.reject")}
            </Button>
          </div>
        </div>

        <div className="min-h-48 rounded-md border bg-background p-3">
          {hasProposal ? (
            view === "diff" ? (
              <AIDiffPreview nextValue={proposal} previousValue={snapshot} />
            ) : (
              <AIProposalPreview format={format} value={proposal} />
            )
          ) : (
            <div className="flex h-full min-h-40 items-center justify-center text-sm text-muted-foreground">
              {isStreaming ? t("ai.streaming") : t("ai.emptyHint")}
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

function AIProposalPreview({
  format,
  value,
}: {
  format: AIProposalFormat;
  value: string;
}) {
  if (format === "html") {
    return (
      <article className="prose prose-sm max-w-none dark:prose-invert">
        <div dangerouslySetInnerHTML={{ __html: value }} />
      </article>
    );
  }

  if (format === "text") {
    return <pre className="whitespace-pre-wrap text-sm leading-6">{value}</pre>;
  }

  return <AIMarkdownPreview markdown={value} />;
}

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === "AbortError";
}

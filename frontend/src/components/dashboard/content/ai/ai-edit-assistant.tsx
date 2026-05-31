"use client";

import {
  Check,
  FileText,
  GitCompareArrows,
  Loader2,
  Sparkles,
  Square,
  X,
} from "lucide-react";
import { useRef, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { AIDiffPreview } from "./ai-diff-preview";
import { AIMarkdownPreview } from "./ai-markdown-preview";

type AIProposalFormat = "html" | "markdown" | "text";

type AIEditAssistantProps = {
  className?: string;
  disabled?: boolean;
  format?: AIProposalFormat;
  onApply: (proposal: string) => void;
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
  const isStreaming = status === "streaming";
  const canGenerate = Boolean(
    !disabled && !isStreaming && message.trim() && source.trim(),
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
      toast.error("AI 编辑失败", {
        description: error instanceof Error ? error.message : "请稍后重试。",
      });
    } finally {
      abortControllerRef.current = null;
    }
  };

  const stop = () => {
    abortControllerRef.current?.abort();
    setStatus(proposal.trim() ? "ready" : "idle");
  };

  const accept = () => {
    if (!proposal.trim()) {
      return;
    }
    onApply(proposal);
    setMessage("");
    setProposal("");
    setStatus("idle");
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
            <Sparkles className="size-4" />
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
            预览
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
            差异
          </Button>
        </div>
      </div>

      <div className="grid gap-3 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
        <div className="space-y-2">
          <Textarea
            value={message}
            onChange={(event) => setMessage(event.target.value)}
            placeholder="让 AI 怎么改"
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
                <Sparkles className="size-4" />
              )}
              生成
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={stop}
              disabled={!isStreaming}
            >
              <Square className="size-4" />
              停止
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={accept}
              disabled={!hasProposal || isStreaming}
            >
              <Check className="size-4" />
              采纳
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={reject}
              disabled={!hasProposal && !isStreaming}
            >
              <X className="size-4" />
              放弃
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
              {isStreaming ? "正在接收..." : "AI 提案会显示在这里"}
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

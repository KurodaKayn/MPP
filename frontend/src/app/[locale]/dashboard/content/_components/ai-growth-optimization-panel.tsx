"use client";

import { Bot, Check, Loader2, Sparkles, X } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { AIDiffPreview } from "@/components/dashboard/content/ai/ai-diff-preview";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { PLATFORM_TABS } from "@/lib/content/platforms";
import type { ContentValue } from "@/lib/content/types";
import {
  applyAIGrowthOptimizationProposal,
  createAIGrowthOptimizationRun,
  rejectAIGrowthOptimizationProposal,
  type AIGrowthOptimizationGoal,
  type AIGrowthOptimizationIntensity,
  type AIGrowthOptimizationRun,
  type AIPlatformProposal,
  type AIProposalStatus,
  type PublishPlatform,
} from "@/lib/dashboard/api";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import { cn } from "@/lib/utils";

type AIGrowthOptimizationPanelProps = {
  canEdit?: boolean;
  content: ContentValue;
  projectId?: string;
  selectedPlatforms: PublishPlatform[];
  title: string;
};

type PanelStatus = "idle" | "running" | "ready" | "applying" | "failed";
type GrowthTranslator = (
  key: string,
  options?: Record<string, unknown>,
) => string;

const goals: { key: string; value: AIGrowthOptimizationGoal }[] = [
  { key: "recommendation", value: "recommendation" },
  { key: "views", value: "views" },
  { key: "ctr", value: "ctr" },
  { key: "completion", value: "completion" },
  { key: "engagement", value: "engagement" },
  { key: "conversion", value: "conversion" },
];

const intensities: { key: string; value: AIGrowthOptimizationIntensity }[] = [
  { key: "conservative", value: "conservative" },
  { key: "balanced", value: "balanced" },
  { key: "aggressive", value: "aggressive" },
];

export function AIGrowthOptimizationPanel({
  canEdit = true,
  content,
  projectId,
  selectedPlatforms,
  title,
}: AIGrowthOptimizationPanelProps) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const { t: tCommon } = useTranslation(locale, "common");
  const tGrowth = (key: string, options?: Record<string, unknown>) =>
    t(`content.aiGrowth.${key}`, options);
  const [goal, setGoal] = useState<AIGrowthOptimizationGoal>("views");
  const [intensity, setIntensity] =
    useState<AIGrowthOptimizationIntensity>("balanced");
  const [status, setStatus] = useState<PanelStatus>("idle");
  const [run, setRun] = useState<AIGrowthOptimizationRun | null>(null);
  const [proposalStatuses, setProposalStatuses] = useState<
    Record<string, AIProposalStatus>
  >({});

  const hasSourceContent = Boolean(content.text.trim() || content.html.trim());
  const targetPlatforms = useMemo<PublishPlatform[]>(
    () => (selectedPlatforms.length > 0 ? selectedPlatforms : ["wechat"]),
    [selectedPlatforms],
  );
  const canOptimize = Boolean(
    canEdit && projectId && hasSourceContent && status !== "running",
  );
  const displayStatus = status === "ready" && run ? run.status : status;
  const canDecideProposals = status === "ready";
  const sourceProposalStatus = run
    ? (proposalStatuses[run.source_proposal.id] ?? run.source_proposal.status)
    : "proposed";

  const runOptimization = async () => {
    if (!projectId || !canOptimize) {
      return;
    }

    setStatus("running");
    setRun(null);
    setProposalStatuses({});

    try {
      const nextRun = await createAIGrowthOptimizationRun(projectId, {
        goal,
        intensity,
        source_content: content.text || content.html,
        target_platforms: targetPlatforms,
        title,
      });
      setRun(nextRun);
      setStatus("ready");
    } catch (error) {
      setStatus("failed");
      toast.error(tGrowth("optimizeFailed"), {
        description:
          error instanceof Error ? error.message : tCommon("common.retryLater"),
      });
    }
  };

  const decideProposal = async (
    proposalId: string,
    decision: Extract<AIProposalStatus, "accepted" | "rejected">,
  ) => {
    if (!projectId) {
      return;
    }

    setStatus("applying");
    try {
      const result =
        decision === "accepted"
          ? await applyAIGrowthOptimizationProposal(projectId, proposalId)
          : await rejectAIGrowthOptimizationProposal(projectId, proposalId);
      setProposalStatuses((current) => ({
        ...current,
        [result.proposal_id]: result.status,
      }));
      toast.success(
        result.status === "accepted"
          ? tGrowth("applySuccess")
          : tGrowth("rejectSuccess"),
      );
      setStatus("ready");
    } catch (error) {
      setStatus("failed");
      toast.error(tGrowth("applyFailed"), {
        description:
          error instanceof Error ? error.message : tCommon("common.retryLater"),
      });
    }
  };

  return (
    <Card>
      <CardHeader className="gap-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Sparkles />
              {tGrowth("title")}
            </CardTitle>
            <CardDescription>{tGrowth("description")}</CardDescription>
          </div>
          <StatusBadge status={displayStatus} tGrowth={tGrowth} />
        </div>

        <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] md:items-end">
          <div className="flex flex-col gap-2">
            <Label htmlFor="ai-growth-goal">{tGrowth("goal")}</Label>
            <Select<AIGrowthOptimizationGoal>
              items={goals.map((item) => ({
                label: tGrowth(`goals.${item.key}`),
                value: item.value,
              }))}
              value={goal}
              disabled={!canEdit || status === "running"}
              onValueChange={(value) => {
                if (value) {
                  setGoal(value);
                }
              }}
            >
              <SelectTrigger id="ai-growth-goal">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {goals.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {tGrowth(`goals.${item.key}`)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="ai-growth-intensity">{tGrowth("intensity")}</Label>
            <Select<AIGrowthOptimizationIntensity>
              items={intensities.map((item) => ({
                label: tGrowth(`intensities.${item.key}`),
                value: item.value,
              }))}
              value={intensity}
              disabled={!canEdit || status === "running"}
              onValueChange={(value) => {
                if (value) {
                  setIntensity(value);
                }
              }}
            >
              <SelectTrigger id="ai-growth-intensity">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {intensities.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {tGrowth(`intensities.${item.key}`)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>

          <Button
            type="button"
            className="w-full md:w-auto"
            disabled={!canOptimize}
            onClick={() => void runOptimization()}
          >
            {status === "running" ? (
              <Loader2 data-icon="inline-start" className="animate-spin" />
            ) : (
              <Bot data-icon="inline-start" />
            )}
            {tGrowth("optimize")}
          </Button>
        </div>

        <div className="flex flex-wrap gap-2">
          {targetPlatforms.map((platform) => (
            <Badge key={platform} variant="outline">
              {platformLabel(platform, tCommon)}
            </Badge>
          ))}
        </div>
      </CardHeader>

      <CardContent className="flex flex-col gap-4">
        {!projectId ? (
          <div className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
            {tGrowth("saveFirst")}
          </div>
        ) : !hasSourceContent ? (
          <div className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
            {tGrowth("emptyContent")}
          </div>
        ) : run ? (
          <>
            <div className="rounded-lg border bg-muted/20 p-3">
              <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <p className="text-sm font-medium">{run.summary}</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {run.model} / {run.prompt_version}
                  </p>
                </div>
                <Badge variant="secondary">
                  {tGrowth("optimizationReady")}
                </Badge>
              </div>
              <WarningList warnings={run.quality_warnings} />
            </div>

            <div className="grid gap-4 xl:grid-cols-[minmax(0,1.05fr)_minmax(0,0.95fr)]">
              <section className="flex flex-col gap-3 rounded-lg border p-3">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div>
                    <h3 className="text-sm font-medium">
                      {tGrowth("sourceProposal")}
                    </h3>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {run.source_proposal.summary}
                    </p>
                  </div>
                  <ProposalDecisionActions
                    disabled={!canDecideProposals}
                    label={tGrowth("source")}
                    status={sourceProposalStatus}
                    tGrowth={tGrowth}
                    onAccept={() =>
                      void decideProposal(run.source_proposal.id, "accepted")
                    }
                    onReject={() =>
                      void decideProposal(run.source_proposal.id, "rejected")
                    }
                  />
                </div>
                <WarningList warnings={run.source_proposal.quality_warnings} />
                <ScrollArea className="h-[320px]">
                  <AIDiffPreview
                    previousValue={run.source_proposal.previous_content}
                    nextValue={run.source_proposal.proposed_content}
                  />
                </ScrollArea>
              </section>

              <section className="flex flex-col gap-3 rounded-lg border p-3">
                <div>
                  <h3 className="text-sm font-medium">
                    {tGrowth("platformProposals")}
                  </h3>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {tGrowth("platformProposalsDesc")}
                  </p>
                </div>
                <Separator />
                <div className="flex flex-col gap-3">
                  {run.platform_proposals.map((proposal) => (
                    <PlatformProposalCard
                      key={proposal.id}
                      disabled={!canDecideProposals}
                      proposal={proposal}
                      status={proposalStatuses[proposal.id] ?? proposal.status}
                      tGrowth={tGrowth}
                      onAccept={() =>
                        void decideProposal(proposal.id, "accepted")
                      }
                      onReject={() =>
                        void decideProposal(proposal.id, "rejected")
                      }
                    />
                  ))}
                </div>
              </section>
            </div>
          </>
        ) : (
          <div className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
            {tGrowth("emptyState")}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function PlatformProposalCard({
  disabled,
  onAccept,
  onReject,
  proposal,
  status,
  tGrowth,
}: {
  disabled: boolean;
  onAccept: () => void;
  onReject: () => void;
  proposal: AIPlatformProposal;
  status: AIProposalStatus;
  tGrowth: GrowthTranslator;
}) {
  const locale = useAppLocale();
  const { t: tCommon } = useTranslation(locale, "common");
  const label = platformLabel(proposal.target_platform, tCommon);

  return (
    <div className="flex flex-col gap-3 rounded-lg border bg-background p-3">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="text-sm font-medium">{label}</h4>
            <ProposalStatusBadge status={status} tGrowth={tGrowth} />
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            {proposal.summary}
          </p>
        </div>
        <ProposalDecisionActions
          disabled={disabled}
          label={label}
          status={status}
          tGrowth={tGrowth}
          onAccept={onAccept}
          onReject={onReject}
        />
      </div>
      <WarningList warnings={proposal.quality_warnings} />
      <div className="rounded-md bg-muted/30 p-3 text-xs leading-5">
        <p className="font-medium">{tGrowth("preview")}</p>
        <p className="mt-2 whitespace-pre-wrap">{proposal.proposed_content}</p>
      </div>
    </div>
  );
}

function ProposalDecisionActions({
  disabled,
  label,
  onAccept,
  onReject,
  status,
  tGrowth,
}: {
  disabled: boolean;
  label: string;
  onAccept: () => void;
  onReject: () => void;
  status: AIProposalStatus;
  tGrowth: GrowthTranslator;
}) {
  const isDecided = status === "accepted" || status === "rejected";

  return (
    <div className="flex shrink-0 flex-wrap gap-2">
      {isDecided ? (
        <ProposalStatusBadge status={status} tGrowth={tGrowth} />
      ) : null}
      <Button
        type="button"
        size="sm"
        disabled={disabled || isDecided}
        onClick={onAccept}
      >
        <Check data-icon="inline-start" />
        {tGrowth("applyLabel", { label })}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={disabled || isDecided}
        onClick={onReject}
      >
        <X data-icon="inline-start" />
        {tGrowth("rejectLabel", { label })}
      </Button>
    </div>
  );
}

function WarningList({
  warnings,
}: {
  warnings: { id: string; message: string; severity: string }[];
}) {
  if (!warnings.length) {
    return null;
  }

  return (
    <div className="mt-3 flex flex-col gap-2">
      {warnings.map((warning) => (
        <div
          key={warning.id}
          className={cn(
            "rounded-md border p-2 text-xs",
            warning.severity === "risk" && "text-destructive",
          )}
        >
          {warning.message}
        </div>
      ))}
    </div>
  );
}

function ProposalStatusBadge({
  status,
  tGrowth,
}: {
  status: AIProposalStatus;
  tGrowth: GrowthTranslator;
}) {
  const label = statusLabel(status, tGrowth);

  return (
    <Badge variant={status === "rejected" ? "outline" : "secondary"}>
      {label}
    </Badge>
  );
}

function StatusBadge({
  status,
  tGrowth,
}: {
  status: PanelStatus | AIGrowthOptimizationRun["status"];
  tGrowth: GrowthTranslator;
}) {
  const label = statusLabel(status, tGrowth);

  return (
    <Badge variant={status === "failed" ? "destructive" : "secondary"}>
      {label}
    </Badge>
  );
}

function statusLabel(status: string, tGrowth: GrowthTranslator) {
  return tGrowth(`status.${status}`);
}

function platformLabel(
  platform: PublishPlatform,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  const platformTab = PLATFORM_TABS.find((item) => item.value === platform);
  if (!platformTab) {
    return platform;
  }

  return t(platformTab.label);
}

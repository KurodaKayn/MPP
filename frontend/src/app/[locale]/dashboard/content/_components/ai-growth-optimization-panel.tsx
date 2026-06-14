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

const goals: { label: string; value: AIGrowthOptimizationGoal }[] = [
  { label: "Recommendations", value: "recommendation" },
  { label: "Reads / views", value: "views" },
  { label: "CTR", value: "ctr" },
  { label: "Completion", value: "completion" },
  { label: "Engagement", value: "engagement" },
  { label: "Conversion", value: "conversion" },
];

const intensities: { label: string; value: AIGrowthOptimizationIntensity }[] = [
  { label: "Conservative", value: "conservative" },
  { label: "Balanced", value: "balanced" },
  { label: "Aggressive", value: "aggressive" },
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
      toast.error(
        t("content.aiGrowth.optimizeFailed", {
          defaultValue: "Could not create optimization preview.",
        }),
        {
          description:
            error instanceof Error
              ? error.message
              : tCommon("common.retryLater", {
                  defaultValue: "Please try again later.",
                }),
        },
      );
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
          ? t("content.aiGrowth.applySuccess", {
              defaultValue: "Optimization proposal applied.",
            })
          : t("content.aiGrowth.rejectSuccess", {
              defaultValue: "Optimization proposal rejected.",
            }),
      );
      setStatus("ready");
    } catch (error) {
      setStatus("failed");
      toast.error(
        t("content.aiGrowth.applyFailed", {
          defaultValue: "Could not update optimization proposal.",
        }),
        {
          description:
            error instanceof Error
              ? error.message
              : tCommon("common.retryLater", {
                  defaultValue: "Please try again later.",
                }),
        },
      );
    }
  };

  return (
    <Card>
      <CardHeader className="gap-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Sparkles />
              {t("content.aiGrowth.title", { defaultValue: "AI Optimize" })}
            </CardTitle>
            <CardDescription>
              {t("content.aiGrowth.description", {
                defaultValue:
                  "Preview growth-focused source and platform proposals before backend optimization is connected.",
              })}
            </CardDescription>
          </div>
          <StatusBadge status={displayStatus} />
        </div>

        <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] md:items-end">
          <div className="flex flex-col gap-2">
            <Label htmlFor="ai-growth-goal">
              {t("content.aiGrowth.goal", { defaultValue: "Goal" })}
            </Label>
            <select
              id="ai-growth-goal"
              value={goal}
              disabled={!canEdit || status === "running"}
              onChange={(event) =>
                setGoal(event.target.value as AIGrowthOptimizationGoal)
              }
              className="h-8 rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
            >
              {goals.map((item) => (
                <option key={item.value} value={item.value}>
                  {item.label}
                </option>
              ))}
            </select>
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="ai-growth-intensity">
              {t("content.aiGrowth.intensity", { defaultValue: "Intensity" })}
            </Label>
            <select
              id="ai-growth-intensity"
              value={intensity}
              disabled={!canEdit || status === "running"}
              onChange={(event) =>
                setIntensity(
                  event.target.value as AIGrowthOptimizationIntensity,
                )
              }
              className="h-8 rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
            >
              {intensities.map((item) => (
                <option key={item.value} value={item.value}>
                  {item.label}
                </option>
              ))}
            </select>
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
            {t("content.aiGrowth.optimize", { defaultValue: "AI Optimize" })}
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
            {t("content.aiGrowth.saveFirst", {
              defaultValue: "Save the project before running AI optimization.",
            })}
          </div>
        ) : !hasSourceContent ? (
          <div className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
            {t("content.aiGrowth.emptyContent", {
              defaultValue:
                "Add source content before running AI optimization.",
            })}
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
                <Badge variant="secondary">Optimization ready</Badge>
              </div>
              <WarningList warnings={run.quality_warnings} />
            </div>

            <div className="grid gap-4 xl:grid-cols-[minmax(0,1.05fr)_minmax(0,0.95fr)]">
              <section className="flex flex-col gap-3 rounded-lg border p-3">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div>
                    <h3 className="text-sm font-medium">
                      {t("content.aiGrowth.sourceProposal", {
                        defaultValue: "Source proposal",
                      })}
                    </h3>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {run.source_proposal.summary}
                    </p>
                  </div>
                  <ProposalDecisionActions
                    disabled={!canDecideProposals}
                    label={t("content.aiGrowth.source", {
                      defaultValue: "Source",
                    })}
                    status={sourceProposalStatus}
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
                    {t("content.aiGrowth.platformProposals", {
                      defaultValue: "Platform proposals",
                    })}
                  </h3>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t("content.aiGrowth.platformProposalsDesc", {
                      defaultValue:
                        "Apply or reject each platform independently.",
                    })}
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
            {t("content.aiGrowth.emptyState", {
              defaultValue:
                "Choose a goal and intensity, then generate a local optimization preview.",
            })}
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
}: {
  disabled: boolean;
  onAccept: () => void;
  onReject: () => void;
  proposal: AIPlatformProposal;
  status: AIProposalStatus;
}) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const { t: tCommon } = useTranslation(locale, "common");
  const label = platformLabel(proposal.target_platform, tCommon);

  return (
    <div className="flex flex-col gap-3 rounded-lg border bg-background p-3">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="text-sm font-medium">{label}</h4>
            <ProposalStatusBadge status={status} />
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            {proposal.summary}
          </p>
        </div>
        <ProposalDecisionActions
          disabled={disabled}
          label={label}
          status={status}
          onAccept={onAccept}
          onReject={onReject}
        />
      </div>
      <WarningList warnings={proposal.quality_warnings} />
      <div className="rounded-md bg-muted/30 p-3 text-xs leading-5">
        <p className="font-medium">
          {t("content.aiGrowth.preview", { defaultValue: "Preview" })}
        </p>
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
}: {
  disabled: boolean;
  label: string;
  onAccept: () => void;
  onReject: () => void;
  status: AIProposalStatus;
}) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const isDecided = status === "accepted" || status === "rejected";

  return (
    <div className="flex shrink-0 flex-wrap gap-2">
      {isDecided ? <ProposalStatusBadge status={status} /> : null}
      <Button
        type="button"
        size="sm"
        disabled={disabled || isDecided}
        onClick={onAccept}
      >
        <Check data-icon="inline-start" />
        {t("content.aiGrowth.applyLabel", {
          defaultValue: `Apply ${label}`,
        })}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={disabled || isDecided}
        onClick={onReject}
      >
        <X data-icon="inline-start" />
        {t("content.aiGrowth.rejectLabel", {
          defaultValue: `Reject ${label}`,
        })}
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

function ProposalStatusBadge({ status }: { status: AIProposalStatus }) {
  const label = status === "accepted" ? "Applied" : statusLabel(status);

  return (
    <Badge variant={status === "rejected" ? "outline" : "secondary"}>
      {label}
    </Badge>
  );
}

function StatusBadge({
  status,
}: {
  status: PanelStatus | AIGrowthOptimizationRun["status"];
}) {
  const label = statusLabel(status);

  return (
    <Badge variant={status === "failed" ? "destructive" : "secondary"}>
      {label}
    </Badge>
  );
}

function statusLabel(status: string) {
  switch (status) {
    case "applying":
      return "Applying";
    case "accepted":
      return "Applied";
    case "cancelled":
      return "Cancelled";
    case "failed":
      return "Failed";
    case "idle":
      return "Idle";
    case "proposed":
      return "Proposed";
    case "ready":
      return "Ready";
    case "rejected":
      return "Rejected";
    case "running":
      return "Running";
    case "superseded":
      return "Superseded";
    default:
      return status;
  }
}

function platformLabel(
  platform: PublishPlatform,
  t: (key: string, options?: { defaultValue?: string }) => string,
) {
  const platformTab = PLATFORM_TABS.find((item) => item.value === platform);
  if (!platformTab) {
    return platform;
  }

  return t(platformTab.label, { defaultValue: platformTab.defaultLabel });
}

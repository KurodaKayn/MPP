import {
  fetchDashboard,
  streamDashboardEvents,
  streamDashboardText,
} from "./client";
import type {
  AIGrowthOptimizationRun,
  AIPlatformProposal,
  AIDraftingReplay,
  AIDraftingStreamOptions,
  AIEditContentStreamInput,
  AIEditPrepublishStreamInput,
  CreateAIGrowthOptimizationRunInput,
  DecideAIProposalResult,
  AITextStreamOptions,
  PublishPlatform,
  ContinueAIDraftingSessionInput,
  StartAIDraftingSessionInput,
} from "./types";

export function streamAIContentEdit(
  input: AIEditContentStreamInput,
  options?: AITextStreamOptions,
) {
  return streamDashboardText(
    "/api/user/dashboard/ai/content/edit/stream",
    input,
    options,
  );
}

export function streamAIPrepublishEdit(
  input: AIEditPrepublishStreamInput,
  options?: AITextStreamOptions,
) {
  return streamDashboardText(
    "/api/user/dashboard/ai/prepublish/edit/stream",
    input,
    options,
  );
}

export async function createAIGrowthOptimizationRun(
  projectId: string,
  input: CreateAIGrowthOptimizationRunInput,
): Promise<AIGrowthOptimizationRun> {
  await Promise.resolve();

  const createdAt = new Date().toISOString();
  const targetPlatforms: PublishPlatform[] =
    input.target_platforms.length > 0 ? input.target_platforms : ["wechat"];
  const sourceProposal = {
    id: `proposal-source-${projectId}`,
    previous_content: input.source_content,
    previous_title: input.title,
    proposed_content: buildOptimizedSource(input.source_content),
    proposed_title: input.title
      ? `${input.title} | Growth Fit`
      : "Growth Fit Draft",
    quality_warnings: [
      {
        id: "source-claims",
        message:
          "Review quantitative claims before publishing; growth optimization should not promise guaranteed platform traffic.",
        severity: "warning" as const,
      },
    ],
    status: "proposed" as const,
    summary:
      "Strengthened the opening hook, clarified the reader payoff, and kept the core argument intact.",
  };

  return {
    created_at: createdAt,
    goal: input.goal,
    id: `mock-growth-run-${projectId}-${createdAt}`,
    intensity: input.intensity,
    model: "mock-growth-ui",
    platform_proposals: targetPlatforms.map((platform) =>
      buildPlatformProposal(platform, input.source_content),
    ),
    project_id: projectId,
    prompt_version: "growth-ui-mock-v1",
    quality_warnings: [
      {
        id: "mock-only",
        message:
          "Backend optimization is not connected yet. This preview uses local mock data for UI validation.",
        severity: "info",
      },
    ],
    source_proposal: sourceProposal,
    status: "ready",
    summary:
      "Optimization ready: source and platform proposals are available for review.",
    target_platforms: targetPlatforms,
    updated_at: createdAt,
  };
}

export async function applyAIGrowthOptimizationProposal(
  _projectId: string,
  proposalId: string,
): Promise<DecideAIProposalResult> {
  await Promise.resolve();

  return {
    proposal_id: proposalId,
    status: "accepted",
  };
}

export async function rejectAIGrowthOptimizationProposal(
  _projectId: string,
  proposalId: string,
): Promise<DecideAIProposalResult> {
  await Promise.resolve();

  return {
    proposal_id: proposalId,
    status: "rejected",
  };
}

function buildOptimizedSource(sourceContent: string) {
  const source = sourceContent.trim() || "Original body";

  return [
    "Optimized body",
    "",
    source,
    "",
    "Reader payoff: clearer positioning, stronger opening momentum, and a more explicit next action.",
  ].join("\n");
}

function buildPlatformProposal(
  platform: PublishPlatform,
  sourceContent: string,
): AIPlatformProposal {
  const platformLabel = platformProposalLabels[platform] ?? platform;
  const previousContent = sourceContent.trim() || "Original body";

  return {
    id: `proposal-${platform}`,
    previous_content: previousContent,
    proposed_content: [
      `Optimized body for ${platformLabel}`,
      "",
      previousContent,
      "",
      platformProposalHints[platform] ??
        "Tightened the hook and clarified the final call to action.",
    ].join("\n"),
    quality_warnings: [
      {
        id: `${platform}-format`,
        message: `${platformLabel} draft should be checked against the final platform editor before publishing.`,
        severity: "warning",
      },
    ],
    status: "proposed",
    summary:
      platformProposalHints[platform] ??
      "Adjusted the draft for platform fit and engagement clarity.",
    target_platform: platform,
  };
}

const platformProposalLabels: Partial<Record<PublishPlatform, string>> = {
  douyin: "Douyin",
  wechat: "WeChat",
  x: "X",
  zhihu: "Zhihu",
};

const platformProposalHints: Partial<Record<PublishPlatform, string>> = {
  douyin:
    "Compressed the caption rhythm and made the interaction cue more direct.",
  wechat:
    "Improved the opening retention and made the closing CTA easier to act on.",
  x: "Shortened the copy for scanning and moved the strongest point earlier.",
  zhihu:
    "Added a more credible argumentative frame and made the structure easier to follow.",
};
export function startAIDraftingSession(
  projectId: string,
  input: StartAIDraftingSessionInput,
  options?: AIDraftingStreamOptions,
) {
  return streamDashboardEvents(
    `/api/user/dashboard/projects/${projectId}/ai/drafting-sessions`,
    input,
    options,
  );
}

export function continueAIDraftingSession(
  sessionId: string,
  input: ContinueAIDraftingSessionInput,
  options?: AIDraftingStreamOptions,
) {
  return streamDashboardEvents(
    `/api/user/dashboard/ai/drafting-sessions/${sessionId}/messages`,
    input,
    options,
  );
}

export function replayAIDraftingSession(sessionId: string) {
  return fetchDashboard<AIDraftingReplay>(
    `/api/user/dashboard/ai/drafting-sessions/${sessionId}/events`,
  );
}

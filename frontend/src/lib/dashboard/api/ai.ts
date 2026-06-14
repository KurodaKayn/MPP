import { fetchDashboard, streamDashboardEvents, streamDashboardText } from "./client";
import type {
  AIDraftingReplay,
  AIDraftingStreamOptions,
  AIEditContentStreamInput,
  AIEditPrepublishStreamInput,
  AITextStreamOptions,
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

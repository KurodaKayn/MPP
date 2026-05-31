import { streamDashboardText } from "./client";
import type {
  AIEditContentStreamInput,
  AIEditPrepublishStreamInput,
  AITextStreamOptions,
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

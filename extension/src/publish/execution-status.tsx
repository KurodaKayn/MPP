import * as React from "react";
import { Badge } from "../components/ui/badge";
import type { ExtensionExecutionEvent } from "../types/events";
import type {
  ExtensionPublishPlatformHandoff,
  StoredHandoff,
} from "../types/handoff";

type BadgeVariant = React.ComponentProps<typeof Badge>["variant"];

export const statusLabels: Record<string, string> = {
  accepted: "accepted",
  opening_tabs: "opening tabs",
  injecting: "injecting",
  user_review: "user review",
  submitted: "submitted",
  succeeded: "succeeded",
  failed: "failed",
  cancelled: "cancelled",
  expired: "expired",
};

const terminalStatuses = new Set([
  "user_review",
  "submitted",
  "succeeded",
  "failed",
  "cancelled",
  "expired",
]);

function getStatusVariant(status?: string): BadgeVariant {
  if (!status) {
    return "secondary";
  }

  if (status === "failed" || status === "expired") {
    return "destructive";
  }

  if (status === "opening_tabs" || status === "injecting") {
    return "warning";
  }

  if (
    status === "accepted" ||
    status === "user_review" ||
    status === "submitted" ||
    status === "succeeded"
  ) {
    return "success";
  }

  return "secondary";
}

export function getStatusLabel(status?: string): string {
  return status ? (statusLabels[status] ?? status) : "idle";
}

export function StatusBadge({ status }: { status?: string }) {
  return (
    <Badge variant={getStatusVariant(status)}>{getStatusLabel(status)}</Badge>
  );
}

export function getLatestPlatformEvent(
  events: ExtensionExecutionEvent[],
  platform: ExtensionPublishPlatformHandoff["platform"],
): ExtensionExecutionEvent | null {
  return (
    events
      .slice()
      .reverse()
      .find((event) => event.platform === platform) ?? null
  );
}

export function CompactExecutionStatus({
  handoff,
  events,
}: {
  handoff: StoredHandoff["handoff"] | null | undefined;
  events: ExtensionExecutionEvent[];
}) {
  if (!handoff) {
    return null;
  }

  const activePlatformEvents = handoff.platforms
    .map((platform) => getLatestPlatformEvent(events, platform.platform))
    .filter((event): event is ExtensionExecutionEvent => event !== null);
  const readyCount = activePlatformEvents.filter((event) =>
    terminalStatuses.has(event.status),
  ).length;
  const latestEvent = events.at(-1);

  return (
    <div className="grid grid-cols-2 gap-3">
      <div className="rounded-lg border border-border bg-card p-3">
        <p className="text-xs text-muted-foreground">Platforms ready</p>
        <p className="mt-1 text-lg font-semibold">
          {readyCount}/{handoff.platforms.length}
        </p>
      </div>
      <div className="rounded-lg border border-border bg-card p-3">
        <p className="text-xs text-muted-foreground">Last event</p>
        <p className="mt-1 text-lg font-semibold">
          {getStatusLabel(latestEvent?.status)}
        </p>
      </div>
    </div>
  );
}

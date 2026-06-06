import * as React from "react";
import {
  AlertCircle,
  CheckCircle2,
  ExternalLink,
  ShieldCheck,
  Trash2,
} from "lucide-react";
import { Alert, AlertDescription } from "../components/ui/alert";
import { Badge } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../components/ui/card";
import { Separator } from "../components/ui/separator";
import type { TrustedOrigin } from "../background/origins";
import type { ExtensionExecutionEvent } from "../types/events";
import type {
  ExtensionPublishPlatformHandoff,
  StoredHandoff,
} from "../types/handoff";
import { getLatestPlatformEvent, StatusBadge } from "./execution-status";
import { SessionStatusCard, type SessionViewState } from "./session";

function getCallbackFailureMessage(
  event: ExtensionExecutionEvent,
): string | null {
  if (!event.metadata.callback_failed) {
    return null;
  }

  const callbackError =
    typeof event.metadata.callback_error === "string"
      ? event.metadata.callback_error
      : "";

  return callbackError
    ? `Callback failed: ${callbackError}`
    : "Callback failed.";
}

function getNextAction(event: ExtensionExecutionEvent | null): string {
  if (!event) {
    return "Waiting for the first execution event.";
  }

  if (event.status === "failed") {
    return "Reopen the platform page and check login, editor, or media upload state.";
  }

  if (event.status === "expired") {
    return "Start a fresh handoff from MPP before continuing.";
  }

  if (event.status === "user_review") {
    return "Review the prepared draft in the platform editor.";
  }

  if (event.status === "opening_tabs" || event.status === "injecting") {
    return "The extension is preparing the platform editor.";
  }

  if (event.status === "accepted") {
    return "The handoff was accepted and tab opening should begin shortly.";
  }

  if (event.status === "succeeded" || event.status === "submitted") {
    return "No follow-up action is required for this platform.";
  }

  return "Reopen the platform page if you need to inspect the draft manually.";
}

function formatDateTime(value: string): string {
  const date = new Date(value);

  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return date.toLocaleString();
}

function ExecutionSummary({
  handoff,
  latestEvent,
  loading,
}: {
  handoff: StoredHandoff["handoff"] | null | undefined;
  latestEvent: ExtensionExecutionEvent | undefined;
  loading: boolean;
}) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <CardTitle>Active Execution</CardTitle>
            <CardDescription>
              {handoff
                ? `Execution ${handoff.execution_id}`
                : loading
                  ? "Loading current execution"
                  : "Waiting for a publishing handoff"}
            </CardDescription>
          </div>
          <StatusBadge status={latestEvent?.status} />
        </div>
      </CardHeader>
      <CardContent>
        <div className="flex flex-col gap-3">
          <div>
            <p className="truncate text-base font-semibold">
              {handoff?.project.title ?? "No active handoff"}
            </p>
            {latestEvent ? (
              <p className="mt-1 text-sm text-muted-foreground">
                {latestEvent.message}
              </p>
            ) : null}
          </div>
          {handoff ? (
            <div className="grid grid-cols-2 gap-2 text-xs text-muted-foreground">
              <div className="rounded-md bg-muted px-3 py-2">
                <p className="font-medium text-foreground">Accepted</p>
                <p>{formatDateTime(handoff.expires_at)}</p>
              </div>
              <div className="rounded-md bg-muted px-3 py-2">
                <p className="font-medium text-foreground">Platforms</p>
                <p>{handoff.platforms.length}</p>
              </div>
            </div>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}

function PlatformCard({
  platform,
  event,
  onReopen,
}: {
  platform: ExtensionPublishPlatformHandoff;
  event: ExtensionExecutionEvent | null;
  onReopen: (platform: ExtensionPublishPlatformHandoff) => void;
}) {
  const callbackMessage = event ? getCallbackFailureMessage(event) : null;
  const showNextAction =
    event?.status === "failed" ||
    event?.status === "expired" ||
    event?.status === "user_review";

  return (
    <div className="rounded-lg border border-border bg-background p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-semibold capitalize">
            {platform.platform}
          </p>
          <p className="mt-1 truncate text-xs text-muted-foreground">
            {platform.adapter_key}
          </p>
        </div>
        <StatusBadge status={event?.status} />
      </div>

      <div className="mt-3 flex flex-col gap-2 text-sm">
        <p className="text-muted-foreground">
          {event?.message ?? "No platform event has been recorded yet."}
        </p>
        {event?.error_message ? (
          <Alert variant="destructive">
            <AlertCircle data-icon="inline-start" />
            <AlertDescription>{event.error_message}</AlertDescription>
          </Alert>
        ) : null}
        {callbackMessage ? (
          <Alert variant="warning">
            <AlertCircle data-icon="inline-start" />
            <AlertDescription>{callbackMessage}</AlertDescription>
          </Alert>
        ) : null}
        {showNextAction ? (
          <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
            {getNextAction(event)}
          </p>
        ) : null}
      </div>

      <div className="mt-3 flex items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">
          {platform.requires_review ? "Review required" : "Review not required"}
        </p>
        <Button variant="outline" onClick={() => onReopen(platform)}>
          <ExternalLink data-icon="inline-start" />
          Reopen
        </Button>
      </div>
    </div>
  );
}

function PlatformStatusList({
  handoff,
  events,
  onReopen,
}: {
  handoff: StoredHandoff["handoff"] | null | undefined;
  events: ExtensionExecutionEvent[];
  onReopen: (platform: ExtensionPublishPlatformHandoff) => void;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Platforms</CardTitle>
        <CardDescription>
          Draft preparation state for each requested platform.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {handoff ? (
          <div className="flex flex-col gap-3">
            {handoff.platforms.map((platform) => (
              <PlatformCard
                key={`${platform.platform}-${platform.adapter_key}`}
                platform={platform}
                event={getLatestPlatformEvent(events, platform.platform)}
                onReopen={onReopen}
              />
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            No platform handoff is active.
          </p>
        )}
      </CardContent>
    </Card>
  );
}

function TrustedOrigins({
  origins,
  onRemove,
}: {
  origins: TrustedOrigin[];
  onRemove: (origin: string) => void;
}) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <ShieldCheck data-icon="inline-start" />
          <CardTitle>Trusted Origins</CardTitle>
        </div>
      </CardHeader>
      <CardContent>
        {origins.length ? (
          <div className="flex flex-col gap-2">
            {origins.map((origin) => (
              <div
                key={origin.origin}
                className="flex items-center justify-between gap-3 rounded-md bg-muted px-3 py-2 text-sm"
              >
                <span className="truncate">{origin.origin}</span>
                <div className="flex shrink-0 items-center gap-2">
                  <CheckCircle2 data-icon="inline-start" />
                  <Button
                    variant="outline"
                    className="size-8 px-0"
                    onClick={() => onRemove(origin.origin)}
                    aria-label={`Remove trusted origin ${origin.origin}`}
                  >
                    <Trash2 data-icon="inline-start" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            No trusted MPP origins yet.
          </p>
        )}
      </CardContent>
    </Card>
  );
}

function EventTimeline({ events }: { events: ExtensionExecutionEvent[] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Execution Events</CardTitle>
        <CardDescription>Newest events are shown first.</CardDescription>
      </CardHeader>
      <CardContent>
        {events.length ? (
          <div className="flex flex-col gap-3">
            {events
              .slice()
              .reverse()
              .map((event, index) => (
                <div key={event.event_id} className="flex flex-col gap-3">
                  <div className="flex items-start gap-3">
                    <div className="mt-1 flex size-3 shrink-0 rounded-full bg-primary" />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <p className="text-sm font-medium capitalize">
                            {event.platform}
                          </p>
                          <p className="mt-1 text-sm text-muted-foreground">
                            {event.message}
                          </p>
                        </div>
                        <StatusBadge status={event.status} />
                      </div>
                      {event.error_message ? (
                        <p className="mt-2 text-xs text-destructive">
                          {event.error_message}
                        </p>
                      ) : null}
                      {getCallbackFailureMessage(event) ? (
                        <p className="mt-2 text-xs text-amber-700">
                          {getCallbackFailureMessage(event)}
                        </p>
                      ) : null}
                      <p className="mt-2 text-xs text-muted-foreground">
                        {formatDateTime(event.created_at)}
                      </p>
                    </div>
                  </div>
                  {index < events.length - 1 ? <Separator /> : null}
                </div>
              ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            No execution events yet.
          </p>
        )}
      </CardContent>
    </Card>
  );
}

export function AccountSettings({
  state,
  onOpenLogin,
  onRetry,
}: {
  state: SessionViewState;
  onOpenLogin: () => void;
  onRetry: () => void;
}) {
  return (
    <section
      className="flex flex-col gap-3"
      aria-labelledby="account-settings-title"
    >
      <div>
        <h2 id="account-settings-title" className="text-sm font-semibold">
          Account Settings
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">
          MPP account state and session actions.
        </p>
      </div>
      <SessionStatusCard
        state={state}
        onOpenLogin={onOpenLogin}
        onRetry={onRetry}
      />
    </section>
  );
}

export function DiagnosticsSettings({
  version,
  handoff,
  events,
  trustedOrigins,
  loading = false,
  onReopen,
  onRemoveOrigin,
  onClearExecutionState,
}: {
  version: string | undefined;
  handoff: StoredHandoff["handoff"] | null | undefined;
  events: ExtensionExecutionEvent[];
  trustedOrigins: TrustedOrigin[];
  loading?: boolean;
  onReopen: (platform: ExtensionPublishPlatformHandoff) => void;
  onRemoveOrigin: (origin: string) => void;
  onClearExecutionState?: () => void;
}) {
  return (
    <section
      className="flex flex-col gap-3"
      aria-labelledby="diagnostics-settings-title"
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 id="diagnostics-settings-title" className="text-sm font-semibold">
            Diagnostics Settings
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Support details for publishing handoffs and trusted origins.
          </p>
        </div>
        {onClearExecutionState ? (
          <Button
            variant="outline"
            onClick={onClearExecutionState}
            aria-label="Clear execution state"
          >
            <Trash2 data-icon="inline-start" />
          </Button>
        ) : null}
      </div>

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between gap-3">
            <div className="min-w-0">
              <CardTitle>Extension Version</CardTitle>
              <CardDescription>Installed publisher build.</CardDescription>
            </div>
            <Badge variant="outline">
              {version ? `v${version}` : "loading"}
            </Badge>
          </div>
        </CardHeader>
      </Card>

      <ExecutionSummary
        handoff={handoff}
        latestEvent={events.at(-1)}
        loading={loading}
      />
      <PlatformStatusList
        handoff={handoff}
        events={events}
        onReopen={onReopen}
      />
      <EventTimeline events={events} />
      <TrustedOrigins origins={trustedOrigins} onRemove={onRemoveOrigin} />
    </section>
  );
}

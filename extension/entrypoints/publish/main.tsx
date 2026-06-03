import React from "react";
import { createRoot } from "react-dom/client";
import {
  AlertCircle,
  CheckCircle2,
  ExternalLink,
  RefreshCw,
  ShieldCheck,
  Trash2,
} from "lucide-react";
import "../../src/styles.css";
import { Alert, AlertDescription } from "../../src/components/ui/alert";
import { Badge } from "../../src/components/ui/badge";
import { Button } from "../../src/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../../src/components/ui/card";
import { Separator } from "../../src/components/ui/separator";
import type { ExtensionExecutionEvent } from "../../src/types/events";
import type {
  ExtensionPublishPlatformHandoff,
  StoredHandoff,
} from "../../src/types/handoff";
import type { BackgroundMessage } from "../../src/types/messages";
import type { TrustedOrigin } from "../../src/background/origins";

interface MonitorState {
  extension_id: string;
  version: string;
  current_handoff: StoredHandoff | null;
  events: ExtensionExecutionEvent[];
  trusted_origins: TrustedOrigin[];
}

type BadgeVariant = React.ComponentProps<typeof Badge>["variant"];

const statusLabels: Record<string, string> = {
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

function sendBackgroundMessage<T>(message: BackgroundMessage): Promise<T> {
  return browser.runtime.sendMessage(message);
}

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

function StatusBadge({ status }: { status?: string }) {
  return (
    <Badge variant={getStatusVariant(status)}>
      {status ? (statusLabels[status] ?? status) : "idle"}
    </Badge>
  );
}

function getLatestPlatformEvent(
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

function useMonitorState() {
  const [state, setState] = React.useState<MonitorState | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState("");

  const load = React.useCallback(async () => {
    try {
      const nextState = await sendBackgroundMessage<MonitorState>({
        type: "monitor.get",
      });
      setState(nextState);
      setError("");
    } catch (nextError) {
      setError(
        nextError instanceof Error ? nextError.message : String(nextError),
      );
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    load();
    const intervalId = window.setInterval(load, 2_000);
    return () => window.clearInterval(intervalId);
  }, [load]);

  return { state, loading, error, setError, load };
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
                  <CheckCircle2 className="size-4 text-emerald-700" />
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

function PublishMonitor() {
  const { state, loading, error, setError, load } = useMonitorState();
  const latestEvent = state?.events.at(-1);
  const handoff = state?.current_handoff?.handoff;

  const clear = async () => {
    await sendBackgroundMessage({ type: "monitor.clear" });
    await load();
  };

  const removeOrigin = async (origin: string) => {
    await sendBackgroundMessage({ type: "origin.remove", origin });
    await load();
  };

  const reopenPlatform = async (platform: ExtensionPublishPlatformHandoff) => {
    try {
      await browser.tabs.create({
        active: true,
        url: platform.inject_url,
      });
      setError("");
    } catch (nextError) {
      setError(
        nextError instanceof Error ? nextError.message : String(nextError),
      );
    }
  };

  const activePlatformEvents =
    handoff?.platforms
      .map((platform) =>
        getLatestPlatformEvent(state?.events ?? [], platform.platform),
      )
      .filter((event): event is ExtensionExecutionEvent => event !== null) ??
    [];
  const readyCount = activePlatformEvents.filter((event) =>
    terminalStatuses.has(event.status),
  ).length;

  return (
    <main className="min-h-screen bg-background">
      <header className="border-b border-border bg-card px-5 py-4">
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0">
            <h1 className="truncate text-lg font-semibold">
              MPP Extension Publisher
            </h1>
            <p className="mt-1 text-xs text-muted-foreground">
              {state ? `v${state.version}` : "Loading extension state"}
            </p>
          </div>
          <div className="flex shrink-0 gap-2">
            <Button variant="outline" onClick={load} aria-label="Refresh">
              <RefreshCw data-icon="inline-start" />
            </Button>
            <Button
              variant="outline"
              onClick={clear}
              aria-label="Clear execution state"
            >
              <Trash2 data-icon="inline-start" />
            </Button>
          </div>
        </div>
      </header>

      <section className="flex flex-col gap-4 px-5 py-5">
        {error ? (
          <Alert variant="destructive">
            <AlertCircle data-icon="inline-start" />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}

        {handoff ? (
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
                {latestEvent ? statusLabels[latestEvent.status] : "none"}
              </p>
            </div>
          </div>
        ) : null}

        <ExecutionSummary
          handoff={handoff}
          latestEvent={latestEvent}
          loading={loading}
        />

        <PlatformStatusList
          handoff={handoff}
          events={state?.events ?? []}
          onReopen={reopenPlatform}
        />

        <EventTimeline events={state?.events ?? []} />

        <TrustedOrigins
          origins={state?.trusted_origins ?? []}
          onRemove={removeOrigin}
        />
      </section>
    </main>
  );
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <PublishMonitor />
  </React.StrictMode>,
);

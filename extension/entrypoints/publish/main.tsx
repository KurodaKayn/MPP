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
import { Button } from "../../src/components/ui/button";
import type { ExtensionExecutionEvent } from "../../src/types/events";
import type { StoredHandoff } from "../../src/types/handoff";
import type { BackgroundMessage } from "../../src/types/messages";
import type { TrustedOrigin } from "../../src/background/origins";
import { cn } from "../../src/lib/cn";

interface MonitorState {
  extension_id: string;
  version: string;
  current_handoff: StoredHandoff | null;
  events: ExtensionExecutionEvent[];
  trusted_origins: TrustedOrigin[];
}

const statusClasses: Record<string, string> = {
  accepted: "bg-sky-100 text-sky-800",
  opening_tabs: "bg-amber-100 text-amber-800",
  injecting: "bg-indigo-100 text-indigo-800",
  user_review: "bg-emerald-100 text-emerald-800",
  submitted: "bg-violet-100 text-violet-800",
  succeeded: "bg-emerald-100 text-emerald-800",
  failed: "bg-red-100 text-red-800",
  cancelled: "bg-zinc-100 text-zinc-700",
  expired: "bg-zinc-100 text-zinc-700",
};

function sendBackgroundMessage<T>(message: BackgroundMessage): Promise<T> {
  return browser.runtime.sendMessage(message);
}

function StatusBadge({ status }: { status: string }) {
  return (
    <span
      className={cn(
        "inline-flex rounded-md px-2 py-1 text-xs font-medium",
        statusClasses[status] ?? "bg-zinc-100 text-zinc-700",
      )}
    >
      {status}
    </span>
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

  return { state, loading, error, load };
}

function PublishMonitor() {
  const { state, loading, error, load } = useMonitorState();
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

  return (
    <main className="min-h-screen">
      <header className="border-b border-zinc-200 bg-white px-5 py-4">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h1 className="text-lg font-semibold text-zinc-950">
              MPP Extension Publisher
            </h1>
            <p className="mt-1 text-xs text-zinc-500">
              {state ? `v${state.version}` : "Loading extension state"}
            </p>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" onClick={load} aria-label="Refresh">
              <RefreshCw size={16} />
            </Button>
            <Button
              variant="outline"
              onClick={clear}
              aria-label="Clear execution state"
            >
              <Trash2 size={16} />
            </Button>
          </div>
        </div>
      </header>

      <section className="space-y-4 px-5 py-5">
        {error ? (
          <div className="flex items-start gap-3 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-800">
            <AlertCircle className="mt-0.5 shrink-0" size={16} />
            <span>{error}</span>
          </div>
        ) : null}

        <section className="rounded-md border border-zinc-200 bg-white p-4">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="text-xs font-medium uppercase text-zinc-500">
                Active Execution
              </p>
              <h2 className="mt-2 text-base font-semibold text-zinc-950">
                {handoff?.project.title ?? "No active handoff"}
              </h2>
              <p className="mt-1 text-sm text-zinc-500">
                {handoff
                  ? `Execution ${handoff.execution_id}`
                  : loading
                    ? "Loading current execution"
                    : "Waiting for MPP to send a publishing handoff"}
              </p>
            </div>
            {latestEvent ? <StatusBadge status={latestEvent.status} /> : null}
          </div>
        </section>

        <section className="rounded-md border border-zinc-200 bg-white p-4">
          <div className="flex items-center gap-2">
            <ShieldCheck size={17} className="text-zinc-600" />
            <h2 className="text-sm font-semibold text-zinc-950">
              Trusted Origins
            </h2>
          </div>
          <div className="mt-3 space-y-2">
            {state?.trusted_origins.length ? (
              state.trusted_origins.map((origin) => (
                <div
                  key={origin.origin}
                  className="flex items-center justify-between gap-3 rounded-md bg-zinc-50 px-3 py-2 text-sm"
                >
                  <span className="truncate">{origin.origin}</span>
                  <div className="flex shrink-0 items-center gap-2">
                    <CheckCircle2 className="text-emerald-600" size={16} />
                    <Button
                      variant="outline"
                      className="h-8 w-8 px-0 text-zinc-500 hover:text-red-700"
                      onClick={() => removeOrigin(origin.origin)}
                      aria-label={`Remove trusted origin ${origin.origin}`}
                    >
                      <Trash2 size={14} />
                    </Button>
                  </div>
                </div>
              ))
            ) : (
              <p className="text-sm text-zinc-500">
                No trusted MPP origins yet.
              </p>
            )}
          </div>
        </section>

        {handoff ? (
          <section className="rounded-md border border-zinc-200 bg-white p-4">
            <h2 className="text-sm font-semibold text-zinc-950">Platforms</h2>
            <div className="mt-3 space-y-3">
              {handoff.platforms.map((platform) => (
                <div
                  key={`${platform.platform}-${platform.adapter_key}`}
                  className="rounded-md bg-zinc-50 p-3"
                >
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <p className="text-sm font-medium text-zinc-950">
                        {platform.platform}
                      </p>
                      <p className="mt-1 text-xs text-zinc-500">
                        {platform.adapter_key}
                      </p>
                    </div>
                    <a
                      className="inline-flex items-center gap-1 text-xs font-medium text-zinc-700 hover:text-zinc-950"
                      href={platform.inject_url}
                      target="_blank"
                      rel="noreferrer"
                    >
                      Open <ExternalLink size={13} />
                    </a>
                  </div>
                  <p className="mt-2 text-xs text-zinc-500">
                    Review required. Auto-publish disabled.
                  </p>
                </div>
              ))}
            </div>
          </section>
        ) : null}

        <section className="rounded-md border border-zinc-200 bg-white p-4">
          <h2 className="text-sm font-semibold text-zinc-950">
            Execution Events
          </h2>
          <div className="mt-3 space-y-3">
            {state?.events.length ? (
              state.events
                .slice()
                .reverse()
                .map((event) => (
                  <div
                    key={event.event_id}
                    className="rounded-md bg-zinc-50 p-3"
                  >
                    {getCallbackFailureMessage(event) ? (
                      <p className="mb-2 text-xs text-amber-700">
                        {getCallbackFailureMessage(event)}
                      </p>
                    ) : null}
                    <div className="flex items-start justify-between gap-3">
                      <div>
                        <p className="text-sm font-medium text-zinc-950">
                          {event.platform}
                        </p>
                        <p className="mt-1 text-sm text-zinc-600">
                          {event.message}
                        </p>
                      </div>
                      <StatusBadge status={event.status} />
                    </div>
                    {event.error_message ? (
                      <p className="mt-2 text-xs text-red-700">
                        {event.error_message}
                      </p>
                    ) : null}
                    <p className="mt-2 text-xs text-zinc-400">
                      {new Date(event.created_at).toLocaleString()}
                    </p>
                  </div>
                ))
            ) : (
              <p className="text-sm text-zinc-500">No execution events yet.</p>
            )}
          </div>
        </section>
      </section>
    </main>
  );
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <PublishMonitor />
  </React.StrictMode>,
);

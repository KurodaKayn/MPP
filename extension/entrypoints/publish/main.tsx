import React from "react";
import { createRoot } from "react-dom/client";
import { AlertCircle, RefreshCw } from "lucide-react";
import "../../src/styles.css";
import { getStoredExtensionAuthToken } from "../../src/backend/auth";
import {
  createBackendClient,
  normalizeBackendError,
} from "../../src/backend/client";
import { backendConfig } from "../../src/backend/config";
import { Alert, AlertDescription } from "../../src/components/ui/alert";
import { Button } from "../../src/components/ui/button";
import type { ExtensionExecutionEvent } from "../../src/types/events";
import type {
  ExtensionPublishPlatformHandoff,
  StoredHandoff,
} from "../../src/types/handoff";
import type {
  BackgroundMessage,
  HandoffResponse,
} from "../../src/types/messages";
import type { PlatformKey } from "../../src/types/platform";
import type { TrustedOrigin } from "../../src/background/origins";
import { useExtensionSession } from "../../src/publish/session";
import {
  PrepublishWorkbenchCard,
  usePrepublishWorkbench,
} from "../../src/publish/prepublish";
import { CompactExecutionStatus } from "../../src/publish/execution-status";
import {
  AccountSettings,
  DiagnosticsSettings,
} from "../../src/publish/settings";

const backendClient = createBackendClient({
  authTokenProvider: getStoredExtensionAuthToken,
});

interface MonitorState {
  extension_id: string;
  version: string;
  current_handoff: StoredHandoff | null;
  events: ExtensionExecutionEvent[];
  trusted_origins: TrustedOrigin[];
}

function sendBackgroundMessage<T>(message: BackgroundMessage): Promise<T> {
  return browser.runtime.sendMessage(message);
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

function PublishMonitor() {
  const { state, loading, error, setError, load } = useMonitorState();
  const { state: sessionState, refresh: refreshSession } = useExtensionSession(
    backendClient.getSession,
  );
  const prepublishWorkbench = usePrepublishWorkbench(
    backendClient.listPrepublish,
    sessionState.status === "authenticated",
  );
  const [startingHandoff, setStartingHandoff] = React.useState(false);
  const [handoffStartError, setHandoffStartError] = React.useState("");
  const handoff = state?.current_handoff?.handoff;

  const refreshAll = async () => {
    await Promise.all([
      load(),
      refreshSession(),
      prepublishWorkbench.onRetry(),
    ]);
  };

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

  const openLogin = async () => {
    try {
      await browser.tabs.create({
        active: true,
        url: backendConfig.loginUrl,
      });
      setError("");
    } catch (nextError) {
      setError(
        nextError instanceof Error ? nextError.message : String(nextError),
      );
    }
  };

  const startSelectedHandoff = React.useCallback(
    async (projectId: string, platforms: PlatformKey[]) => {
      try {
        setStartingHandoff(true);
        setHandoffStartError("");

        const handoffResponse = await backendClient.createHandoff({
          project_id: projectId,
          platforms,
        });
        const response = await sendBackgroundMessage<HandoffResponse>({
          type: "extension.start_handoff",
          handoff: handoffResponse,
        });

        if (!response.accepted) {
          throw new Error(response.message);
        }

        await load();
      } catch (nextError) {
        setHandoffStartError(normalizeBackendError(nextError).message);
      } finally {
        setStartingHandoff(false);
      }
    },
    [load],
  );

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
            <Button variant="outline" onClick={refreshAll} aria-label="Refresh">
              <RefreshCw data-icon="inline-start" />
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

        <PrepublishWorkbenchCard
          {...prepublishWorkbench}
          onStartHandoff={startSelectedHandoff}
          startingHandoff={startingHandoff}
          startError={handoffStartError}
        />

        <CompactExecutionStatus
          handoff={handoff}
          events={state?.events ?? []}
        />

        <AccountSettings
          state={sessionState}
          onOpenLogin={openLogin}
          onRetry={refreshSession}
        />

        <DiagnosticsSettings
          version={state?.version}
          handoff={handoff}
          events={state?.events ?? []}
          trustedOrigins={state?.trusted_origins ?? []}
          loading={loading}
          onReopen={reopenPlatform}
          onRemoveOrigin={removeOrigin}
          onClearExecutionState={clear}
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

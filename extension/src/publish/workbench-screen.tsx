import * as React from "react";
import { AlertCircle, RefreshCw, Settings, X } from "lucide-react";
import { Alert, AlertDescription } from "../components/ui/alert";
import { Button } from "../components/ui/button";
import type { TrustedOrigin } from "../background/origins";
import type { ExtensionExecutionEvent } from "../types/events";
import type {
  ExtensionPublishPlatformHandoff,
  StoredHandoff,
} from "../types/handoff";
import type { PlatformKey } from "../types/platform";
import { AccountSettings, DiagnosticsSettings } from "./settings";
import { CompactExecutionStatus } from "./execution-status";
import {
  PrepublishWorkbenchCard,
  type PrepublishWorkbenchProps,
} from "./prepublish";
import type { SessionViewState } from "./session";

export interface PublishWorkbenchScreenProps {
  error: string;
  version?: string;
  handoff: StoredHandoff["handoff"] | null | undefined;
  events: ExtensionExecutionEvent[];
  prepublishWorkbench: PrepublishWorkbenchProps;
  startingHandoff: boolean;
  handoffStartError: string;
  sessionState: SessionViewState;
  trustedOrigins: TrustedOrigin[];
  settingsLoading: boolean;
  onRefresh: () => void;
  onOpenLogin: () => void;
  onRefreshSession: () => void;
  onStartHandoff: (projectId: string, platforms: PlatformKey[]) => void;
  onReopenPlatform: (platform: ExtensionPublishPlatformHandoff) => void;
  onRemoveOrigin: (origin: string) => void;
  onClearExecutionState: () => void;
}

export function PublishWorkbenchScreen({
  error,
  version,
  handoff,
  events,
  prepublishWorkbench,
  startingHandoff,
  handoffStartError,
  sessionState,
  trustedOrigins,
  settingsLoading,
  onRefresh,
  onOpenLogin,
  onRefreshSession,
  onStartHandoff,
  onReopenPlatform,
  onRemoveOrigin,
  onClearExecutionState,
}: PublishWorkbenchScreenProps) {
  const [settingsOpen, setSettingsOpen] = React.useState(false);
  const [diagnosticsOpen, setDiagnosticsOpen] = React.useState(false);

  return (
    <main className="min-h-screen bg-background">
      <header className="border-b border-border bg-card px-5 py-4">
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0">
            <h1 className="truncate text-lg font-semibold">MPP Publisher</h1>
          </div>
          <div className="flex shrink-0 gap-2">
            <Button variant="outline" onClick={onRefresh} aria-label="Refresh">
              <RefreshCw data-icon="inline-start" />
            </Button>
            <Button
              variant="outline"
              onClick={() => setSettingsOpen(true)}
              aria-label="Open settings"
            >
              <Settings data-icon="inline-start" />
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
          onOpenLogin={onOpenLogin}
          onStartHandoff={onStartHandoff}
          startingHandoff={startingHandoff}
          startError={handoffStartError}
        />

        <CompactExecutionStatus handoff={handoff} events={events} />
      </section>

      {settingsOpen ? (
        <div className="fixed inset-0 bg-background/80">
          <aside
            role="dialog"
            aria-modal="true"
            aria-labelledby="settings-panel-title"
            className="ml-auto flex h-full w-full max-w-xl flex-col gap-4 overflow-auto border-l border-border bg-background p-5 shadow-lg"
          >
            <div className="flex items-center justify-between gap-3">
              <h2 id="settings-panel-title" className="text-lg font-semibold">
                Settings
              </h2>
              <Button
                variant="outline"
                onClick={() => setSettingsOpen(false)}
                aria-label="Close settings"
              >
                <X data-icon="inline-start" />
              </Button>
            </div>

            <AccountSettings
              state={sessionState}
              onOpenLogin={onOpenLogin}
              onRetry={onRefreshSession}
            />

            <div className="flex justify-end">
              <Button variant="outline" onClick={onRefreshSession}>
                <RefreshCw data-icon="inline-start" />
                Refresh Session
              </Button>
            </div>

            <section className="flex flex-col gap-3">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <h3 className="text-sm font-semibold">Diagnostics</h3>
                </div>
                <Button
                  variant="outline"
                  onClick={() => setDiagnosticsOpen((current) => !current)}
                >
                  {diagnosticsOpen ? "Hide Diagnostics" : "Show Diagnostics"}
                </Button>
              </div>

              {diagnosticsOpen ? (
                <DiagnosticsSettings
                  version={version}
                  handoff={handoff}
                  events={events}
                  trustedOrigins={trustedOrigins}
                  loading={settingsLoading}
                  onReopen={onReopenPlatform}
                  onRemoveOrigin={onRemoveOrigin}
                  onClearExecutionState={onClearExecutionState}
                />
              ) : null}
            </section>
          </aside>
        </div>
      ) : null}
    </main>
  );
}

import * as React from "react";
import { AlertCircle, RefreshCw } from "lucide-react";
import { Alert, AlertDescription } from "../components/ui/alert";
import { Button } from "../components/ui/button";
import type { ExtensionExecutionEvent } from "../types/events";
import type { StoredHandoff } from "../types/handoff";
import type { PlatformKey } from "../types/platform";
import { CompactExecutionStatus } from "./execution-status";
import {
  PrepublishWorkbenchCard,
  type PrepublishWorkbenchProps,
} from "./prepublish";

export interface PublishWorkbenchScreenProps {
  error: string;
  version?: string;
  handoff: StoredHandoff["handoff"] | null | undefined;
  events: ExtensionExecutionEvent[];
  prepublishWorkbench: PrepublishWorkbenchProps;
  startingHandoff: boolean;
  handoffStartError: string;
  onRefresh: () => void;
  onOpenLogin: () => void;
  onStartHandoff: (projectId: string, platforms: PlatformKey[]) => void;
}

export function PublishWorkbenchScreen({
  error,
  handoff,
  events,
  prepublishWorkbench,
  startingHandoff,
  handoffStartError,
  onRefresh,
  onOpenLogin,
  onStartHandoff,
}: PublishWorkbenchScreenProps) {
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
    </main>
  );
}

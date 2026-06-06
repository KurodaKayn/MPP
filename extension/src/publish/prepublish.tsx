import * as React from "react";
import {
  AlertCircle,
  CheckCircle2,
  ExternalLink,
  FileText,
  Play,
  RefreshCw,
} from "lucide-react";
import { normalizeBackendError } from "../backend/client";
import type {
  ExtensionPrepublishItem,
  ExtensionPrepublishPlatform,
  ExtensionPrepublishResponse,
} from "../backend/types";
import type { PlatformKey } from "../types/platform";
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

export type PrepublishViewState =
  | {
      status: "idle";
    }
  | {
      status: "loading";
    }
  | {
      status: "empty";
    }
  | {
      status: "error";
      message: string;
    }
  | {
      status: "loaded";
      items: ExtensionPrepublishItem[];
    };

export type LoadPrepublish = () => Promise<ExtensionPrepublishResponse>;

export interface PrepublishWorkbenchProps {
  state: PrepublishViewState;
  selectedProjectId: string | null;
  selectedPlatforms: Set<PlatformKey>;
  onProjectSelect: (projectId: string) => void;
  onPlatformToggle: (platform: PlatformKey) => void;
  onRetry: () => void;
  onOpenLogin?: () => void;
  onStartHandoff?: (projectId: string, platforms: PlatformKey[]) => void;
  startingHandoff?: boolean;
  startError?: string;
}

export async function getPrepublishViewState(
  loadPrepublish: LoadPrepublish,
): Promise<PrepublishViewState> {
  try {
    const prepublish = await loadPrepublish();

    if (prepublish.items.length === 0) {
      return { status: "empty" };
    }

    return {
      status: "loaded",
      items: prepublish.items,
    };
  } catch (error) {
    return {
      status: "error",
      message: normalizeBackendError(error).message,
    };
  }
}

function enabledPlatformsForProject(
  item: ExtensionPrepublishItem | undefined,
): Set<PlatformKey> {
  return new Set(
    item?.platforms
      .filter((platform) => platform.enabled)
      .map((platform) => platform.platform) ?? [],
  );
}

function getWorkbenchStatusLabel(
  status: PrepublishViewState["status"],
): string {
  if (status === "idle") {
    return "sign in";
  }

  if (status === "error") {
    return "attention";
  }

  if (status === "loaded") {
    return "ready";
  }

  return status;
}

function getWorkbenchStatusVariant(
  status: PrepublishViewState["status"],
): React.ComponentProps<typeof Badge>["variant"] {
  if (status === "loaded") {
    return "success";
  }

  if (status === "loading") {
    return "info";
  }

  if (status === "error") {
    return "warning";
  }

  return "secondary";
}

function formatSelectedPlatformCount(count: number): string {
  return `${count} ${count === 1 ? "platform" : "platforms"} selected`;
}

function getPlatformAvailabilityLabel(
  platform: ExtensionPrepublishPlatform,
): string {
  return platform.enabled ? "ready" : "unavailable";
}

export function usePrepublishWorkbench(
  loadPrepublish: LoadPrepublish,
  enabled: boolean,
): PrepublishWorkbenchProps {
  const [state, setState] = React.useState<PrepublishViewState>(
    enabled ? { status: "loading" } : { status: "idle" },
  );
  const [selectedProjectId, setSelectedProjectId] = React.useState<
    string | null
  >(null);
  const [selectedPlatforms, setSelectedPlatforms] = React.useState<
    Set<PlatformKey>
  >(new Set());

  const selectProject = React.useCallback(
    (projectId: string, items?: ExtensionPrepublishItem[]) => {
      const sourceItems =
        items ?? (state.status === "loaded" ? state.items : []);
      const nextProject = sourceItems.find(
        (item) => item.project_id === projectId,
      );

      setSelectedProjectId(projectId);
      setSelectedPlatforms(enabledPlatformsForProject(nextProject));
    },
    [state],
  );

  const refresh = React.useCallback(async () => {
    if (!enabled) {
      setState({ status: "idle" });
      setSelectedProjectId(null);
      setSelectedPlatforms(new Set());
      return;
    }

    setState({ status: "loading" });
    const nextState = await getPrepublishViewState(loadPrepublish);
    setState(nextState);

    if (nextState.status === "loaded") {
      const firstProject = nextState.items[0];
      setSelectedProjectId(firstProject.project_id);
      setSelectedPlatforms(enabledPlatformsForProject(firstProject));
      return;
    }

    setSelectedProjectId(null);
    setSelectedPlatforms(new Set());
  }, [enabled, loadPrepublish]);

  const togglePlatform = React.useCallback(
    (platform: PlatformKey) => {
      setSelectedPlatforms((current) => {
        const next = new Set(current);

        if (next.has(platform)) {
          next.delete(platform);
        } else {
          next.add(platform);
        }

        return next;
      });
    },
    [setSelectedPlatforms],
  );

  React.useEffect(() => {
    refresh();
  }, [refresh]);

  return {
    state,
    selectedProjectId,
    selectedPlatforms,
    onProjectSelect: selectProject,
    onPlatformToggle: togglePlatform,
    onRetry: refresh,
  };
}

function formatDateTime(value: string): string {
  const date = new Date(value);

  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return date.toLocaleString();
}

function ProjectList({
  items,
  selectedProjectId,
  onProjectSelect,
}: {
  items: ExtensionPrepublishItem[];
  selectedProjectId: string | null;
  onProjectSelect: (projectId: string) => void;
}) {
  return (
    <div className="flex flex-col gap-2">
      {items.map((item) => {
        const selected = item.project_id === selectedProjectId;

        return (
          <button
            key={item.project_id}
            type="button"
            className={[
              "flex w-full items-start gap-3 rounded-md border px-3 py-2 text-left transition-colors",
              selected
                ? "border-primary bg-primary/5"
                : "border-border bg-background hover:bg-muted",
            ].join(" ")}
            onClick={() => onProjectSelect(item.project_id)}
          >
            <FileText className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-medium">
                {item.title}
              </span>
              <span className="mt-1 block text-xs text-muted-foreground">
                {formatDateTime(item.updated_at)}
              </span>
            </span>
            {selected ? (
              <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-emerald-700" />
            ) : null}
          </button>
        );
      })}
    </div>
  );
}

function PlatformSelection({
  platforms,
  selectedPlatforms,
  onPlatformToggle,
}: {
  platforms: ExtensionPrepublishPlatform[];
  selectedPlatforms: Set<PlatformKey>;
  onPlatformToggle: (platform: PlatformKey) => void;
}) {
  return (
    <div className="flex flex-col gap-2">
      {platforms.map((platform) => (
        <label
          key={platform.publication_id}
          className={[
            "flex items-start gap-3 rounded-md border px-3 py-2",
            platform.enabled
              ? "border-border bg-background"
              : "border-border bg-muted opacity-70",
          ].join(" ")}
        >
          <input
            type="checkbox"
            className="mt-1"
            checked={
              platform.enabled && selectedPlatforms.has(platform.platform)
            }
            disabled={!platform.enabled}
            onChange={() => onPlatformToggle(platform.platform)}
            aria-label={platform.platform}
          />
          <span className="min-w-0 flex-1">
            <span className="flex items-center justify-between gap-2">
              <span className="text-sm font-medium capitalize">
                {platform.platform}
              </span>
              <Badge variant={platform.enabled ? "success" : "secondary"}>
                {getPlatformAvailabilityLabel(platform)}
              </Badge>
            </span>
            {platform.preview ? (
              <span className="mt-2 block text-sm text-muted-foreground">
                {platform.preview}
              </span>
            ) : null}
          </span>
        </label>
      ))}
    </div>
  );
}

function LoadedWorkbench({
  state,
  selectedProjectId,
  selectedPlatforms,
  onProjectSelect,
  onPlatformToggle,
  onStartHandoff,
  startingHandoff = false,
  startError = "",
}: PrepublishWorkbenchProps & {
  state: Extract<PrepublishViewState, { status: "loaded" }>;
}) {
  const selectedProject =
    state.items.find((item) => item.project_id === selectedProjectId) ??
    state.items[0];
  const selectedPlatformList = selectedProject.platforms
    .filter(
      (platform) =>
        platform.enabled && selectedPlatforms.has(platform.platform),
    )
    .map((platform) => platform.platform);
  const canStart =
    Boolean(onStartHandoff) &&
    Boolean(selectedProject.project_id) &&
    selectedPlatformList.length > 0 &&
    !startingHandoff;

  return (
    <div className="flex flex-col gap-4">
      <ProjectList
        items={state.items}
        selectedProjectId={selectedProject.project_id}
        onProjectSelect={onProjectSelect}
      />
      <div className="rounded-md border border-border bg-muted/50 p-3">
        <div className="mb-3 flex items-center justify-between gap-3">
          <div className="min-w-0">
            <p className="truncate text-sm font-medium">
              {selectedProject.title}
            </p>
            <p className="mt-1 text-xs text-muted-foreground">
              {formatSelectedPlatformCount(selectedPlatformList.length)}
            </p>
          </div>
          <Badge variant={canStart ? "info" : "secondary"}>
            {canStart ? "ready" : "choose platform"}
          </Badge>
        </div>
        <PlatformSelection
          platforms={selectedProject.platforms}
          selectedPlatforms={selectedPlatforms}
          onPlatformToggle={onPlatformToggle}
        />
        {startError ? (
          <Alert variant="destructive" className="mt-3">
            <AlertCircle data-icon="inline-start" />
            <AlertDescription>{startError}</AlertDescription>
          </Alert>
        ) : null}
        {!selectedPlatformList.length ? (
          <p className="mt-3 text-sm text-muted-foreground">
            Select at least one platform.
          </p>
        ) : null}
        <div className="mt-3 flex justify-end">
          <Button
            type="button"
            disabled={!canStart}
            onClick={() =>
              onStartHandoff?.(selectedProject.project_id, selectedPlatformList)
            }
          >
            <Play data-icon="inline-start" />
            {startingHandoff ? "Starting Publishing" : "Start Publishing"}
          </Button>
        </div>
      </div>
    </div>
  );
}

export function PrepublishWorkbenchCard(props: PrepublishWorkbenchProps) {
  const { state, onRetry, onOpenLogin } = props;

  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <CardTitle>Pre-Publish Drafts</CardTitle>
            <CardDescription>
              Choose a draft and platform to prepare.
            </CardDescription>
          </div>
          <Badge variant={getWorkbenchStatusVariant(state.status)}>
            {getWorkbenchStatusLabel(state.status)}
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        {state.status === "idle" ? (
          <div className="flex flex-col gap-3">
            <p className="text-sm text-muted-foreground">
              Sign in to MPP to load drafts.
            </p>
            <div className="flex flex-wrap gap-2">
              {onOpenLogin ? (
                <Button onClick={onOpenLogin}>
                  <ExternalLink data-icon="inline-start" />
                  Open MPP
                </Button>
              ) : null}
              <Button variant="outline" onClick={onRetry}>
                <RefreshCw data-icon="inline-start" />
                Retry
              </Button>
            </div>
          </div>
        ) : null}
        {state.status === "loading" ? (
          <p className="text-sm text-muted-foreground">Loading drafts.</p>
        ) : null}
        {state.status === "empty" ? (
          <p className="text-sm text-muted-foreground">
            No pre-publish drafts yet.
          </p>
        ) : null}
        {state.status === "error" ? (
          <div className="flex flex-col gap-3">
            <Alert variant="warning">
              <AlertCircle data-icon="inline-start" />
              <AlertDescription>{state.message}</AlertDescription>
            </Alert>
            <Button variant="outline" onClick={onRetry}>
              <RefreshCw data-icon="inline-start" />
              Retry
            </Button>
          </div>
        ) : null}
        {state.status === "loaded" ? (
          <LoadedWorkbench {...props} state={state} />
        ) : null}
      </CardContent>
    </Card>
  );
}

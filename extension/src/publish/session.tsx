import * as React from "react";
import {
  AlertCircle,
  CheckCircle2,
  ExternalLink,
  Loader2,
  RefreshCw,
} from "lucide-react";
import { normalizeBackendError } from "../backend/client";
import type {
  ExtensionSessionResponse,
  ExtensionSessionUser,
} from "../backend/types";
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

export type SessionViewState =
  | {
      status: "loading";
    }
  | {
      status: "authenticated";
      user: ExtensionSessionUser;
    }
  | {
      status: "unauthenticated" | "expired" | "api_unavailable";
      message: string;
    };

export type LoadExtensionSession = () => Promise<ExtensionSessionResponse>;

function getFailureState(error: unknown): SessionViewState {
  const normalizedError = normalizeBackendError(error);

  if (
    normalizedError.status === 401 ||
    normalizedError.code === "missing_auth_token"
  ) {
    return {
      status: "expired",
      message: normalizedError.message,
    };
  }

  return {
    status: "api_unavailable",
    message: normalizedError.message,
  };
}

export async function getSessionViewState(
  loadSession: LoadExtensionSession,
): Promise<SessionViewState> {
  try {
    const session = await loadSession();

    if (session.authenticated) {
      return {
        status: "authenticated",
        user: session.user,
      };
    }

    return {
      status: "unauthenticated",
      message: "MPP login is required.",
    };
  } catch (error) {
    return getFailureState(error);
  }
}

export function useExtensionSession(loadSession: LoadExtensionSession) {
  const [state, setState] = React.useState<SessionViewState>({
    status: "loading",
  });

  const refresh = React.useCallback(async () => {
    setState({ status: "loading" });
    setState(await getSessionViewState(loadSession));
  }, [loadSession]);

  React.useEffect(() => {
    refresh();
  }, [refresh]);

  return { state, refresh };
}

function sessionBadgeVariant(
  state: SessionViewState,
): React.ComponentProps<typeof Badge>["variant"] {
  if (state.status === "authenticated") {
    return "success";
  }

  if (state.status === "loading") {
    return "info";
  }

  if (state.status === "api_unavailable") {
    return "warning";
  }

  return "destructive";
}

function sessionBadgeLabel(state: SessionViewState): string {
  if (state.status === "authenticated") {
    return "connected";
  }

  if (state.status === "loading") {
    return "checking";
  }

  if (state.status === "api_unavailable") {
    return "offline";
  }

  if (state.status === "expired") {
    return "expired";
  }

  return "signed out";
}

function SessionBody({
  state,
  onOpenLogin,
  onRetry,
}: {
  state: SessionViewState;
  onOpenLogin: () => void;
  onRetry: () => void;
}) {
  if (state.status === "loading") {
    return (
      <div className="flex items-center gap-3 rounded-md bg-muted px-3 py-2 text-sm text-muted-foreground">
        <Loader2 data-icon="inline-start" className="animate-spin" />
        <span>Checking MPP session</span>
      </div>
    );
  }

  if (state.status === "authenticated") {
    return (
      <div className="flex items-center justify-between gap-3 rounded-md bg-muted px-3 py-2">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">{state.user.username}</p>
          <p className="text-xs text-muted-foreground">MPP Web session</p>
        </div>
        <CheckCircle2 className="size-4 shrink-0 text-emerald-700" />
      </div>
    );
  }

  if (state.status === "api_unavailable") {
    return (
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
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <Alert variant="destructive">
        <AlertCircle data-icon="inline-start" />
        <AlertDescription>{state.message}</AlertDescription>
      </Alert>
      <Button onClick={onOpenLogin}>
        <ExternalLink data-icon="inline-start" />
        Open MPP
      </Button>
    </div>
  );
}

export function SessionStatusCard({
  state,
  onOpenLogin,
  onRetry,
}: {
  state: SessionViewState;
  onOpenLogin: () => void;
  onRetry: () => void;
}) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <CardTitle>MPP Session</CardTitle>
            <CardDescription>Authenticated backend access</CardDescription>
          </div>
          <Badge variant={sessionBadgeVariant(state)}>
            {sessionBadgeLabel(state)}
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        <SessionBody
          state={state}
          onOpenLogin={onOpenLogin}
          onRetry={onRetry}
        />
      </CardContent>
    </Card>
  );
}

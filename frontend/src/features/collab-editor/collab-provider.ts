import { HocuspocusProvider } from "@hocuspocus/provider";
import type * as Y from "yjs";
import type {
  CollabDocumentRole,
  CollabDocumentSession,
} from "@/lib/dashboard/api";

export type CollabConnectionStatus =
  | "idle"
  | "connecting"
  | "connected"
  | "synced"
  | "offline"
  | "unauthorized"
  | "error";

export type CollabUserProfile = {
  color: string;
  name: string;
  role: CollabDocumentRole;
};

type AwarenessState = {
  clientId: number;
  color?: string;
  name?: string;
  role?: CollabDocumentRole;
  user?: Partial<CollabUserProfile>;
};

type CreateCollabProviderOptions = {
  documentId: string;
  onAuthenticationFailed?: (reason: string) => void;
  onStatusChange?: (status: CollabConnectionStatus) => void;
  onSyncedChange?: (isSynced: boolean) => void;
  onUnsyncedChanges?: (count: number) => void;
  onUsersChange?: (users: CollabUserProfile[]) => void;
  session: CollabDocumentSession;
  ydoc: Y.Doc;
};

type DecorationAttrs = Record<string, string>;

const userColors = [
  "#2563eb",
  "#16a34a",
  "#dc2626",
  "#9333ea",
  "#ea580c",
  "#0891b2",
  "#be123c",
  "#4f46e5",
] as const;

export function resolveCollabWebSocketUrl(
  websocketUrl: string,
  origin = typeof window === "undefined" ? "" : window.location.origin,
) {
  const trimmedUrl = websocketUrl.trim();

  if (/^wss?:\/\//i.test(trimmedUrl)) {
    return trimmedUrl;
  }

  if (!origin) {
    return trimmedUrl;
  }

  const baseUrl = new URL(origin);
  baseUrl.protocol = baseUrl.protocol === "https:" ? "wss:" : "ws:";
  return new URL(trimmedUrl, baseUrl).toString();
}

export function getCollabUserProfile(
  name: string | null | undefined,
  role: CollabDocumentRole,
  seed: string,
): CollabUserProfile {
  const normalizedName = name?.trim() || "Collaborator";

  return {
    color:
      userColors[hashString(`${seed}:${normalizedName}`) % userColors.length],
    name: normalizedName,
    role,
  };
}

export function createCollabProvider({
  documentId,
  onAuthenticationFailed,
  onStatusChange,
  onSyncedChange,
  onUnsyncedChanges,
  onUsersChange,
  session,
  ydoc,
}: CreateCollabProviderOptions) {
  return new HocuspocusProvider({
    document: ydoc,
    name: documentId,
    onAuthenticationFailed: ({ reason }) => {
      onAuthenticationFailed?.(reason);
    },
    onAwarenessChange: ({ states }) => {
      onUsersChange?.(normalizeAwarenessUsers(states as AwarenessState[]));
    },
    onStatus: ({ status }) => {
      onStatusChange?.(status === "connected" ? "connected" : "offline");
    },
    onSynced: ({ state }) => {
      onSyncedChange?.(state);
    },
    onUnsyncedChanges: ({ number }) => {
      onUnsyncedChanges?.(number);
    },
    token: session.token,
    url: resolveCollabWebSocketUrl(session.websocket_url),
  });
}

export function renderCollabCursor(user: Record<string, unknown>) {
  const color = typeof user.color === "string" ? user.color : "#2563eb";
  const name = typeof user.name === "string" ? user.name : "Collaborator";
  const cursor = document.createElement("span");
  const label = document.createElement("span");

  cursor.classList.add("collab-cursor");
  cursor.style.borderColor = color;
  label.classList.add("collab-cursor-label");
  label.style.backgroundColor = color;
  label.textContent = name;
  cursor.append(label);

  return cursor;
}

export function renderCollabSelection(
  user: Record<string, unknown>,
): DecorationAttrs {
  const color = typeof user.color === "string" ? user.color : "#2563eb";
  const name = typeof user.name === "string" ? user.name : "Collaborator";

  return {
    "data-user": name,
    nodeName: "span",
    style: `background-color: ${color}30`,
  };
}

function normalizeAwarenessUsers(
  states: AwarenessState[],
): CollabUserProfile[] {
  return states.flatMap((state) => {
    const candidate = state.user ?? state;

    if (typeof candidate.name !== "string") {
      return [];
    }

    const profile: CollabUserProfile = {
      color: typeof candidate.color === "string" ? candidate.color : "#2563eb",
      name: candidate.name,
      role: candidate.role === "viewer" ? "viewer" : "editor",
    };

    return [profile];
  });
}

function hashString(value: string) {
  let hash = 0;

  for (const character of value) {
    hash = (hash * 31 + character.charCodeAt(0)) >>> 0;
  }

  return hash;
}

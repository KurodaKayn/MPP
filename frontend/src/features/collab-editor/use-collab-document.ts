"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import * as Y from "yjs";
import {
  createCollabDocument,
  createCollabDocumentSession,
  listCollabDocuments,
  updateCollabDocument,
  type CollabDocument,
  type CollabDocumentRole,
} from "@/lib/dashboard/api";
import {
  createCollabProvider,
  getCollabUserProfile,
  type CollabConnectionStatus,
  type CollabUserProfile,
} from "./collab-provider";

type UseCollabDocumentsOptions = {
  defaultTitle: string;
  fallbackError: string;
  onError: (message: string) => void;
};

type UseCollabConnectionOptions = {
  document: CollabDocument | null;
  userName: string;
};

export function useCollabDocuments({
  defaultTitle,
  fallbackError,
  onError,
}: UseCollabDocumentsOptions) {
  const [documents, setDocuments] = useState<CollabDocument[]>([]);
  const [selectedDocumentId, setSelectedDocumentId] = useState<string | null>(
    null,
  );
  const [isCreating, setIsCreating] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [isRenaming, setIsRenaming] = useState(false);
  const [error, setError] = useState("");

  const selectedDocument = useMemo(
    () =>
      documents.find((document) => document.id === selectedDocumentId) ?? null,
    [documents, selectedDocumentId],
  );

  const loadDocuments = useCallback(async () => {
    setIsLoading(true);
    setError("");

    try {
      const response = await listCollabDocuments({ limit: 50 });
      setDocuments(response.items);
      setSelectedDocumentId((currentDocumentId) => {
        if (
          currentDocumentId &&
          response.items.some((document) => document.id === currentDocumentId)
        ) {
          return currentDocumentId;
        }

        return response.items[0]?.id ?? null;
      });
    } catch (requestError) {
      const message = getErrorMessage(requestError, fallbackError);
      setError(message);
      onError(message);
    } finally {
      setIsLoading(false);
    }
  }, [fallbackError, onError]);

  useEffect(() => {
    void loadDocuments();
  }, [loadDocuments]);

  const createDocument = useCallback(
    async (title: string) => {
      const normalizedTitle = title.trim() || defaultTitle;
      setIsCreating(true);

      try {
        const document = await createCollabDocument({
          title: normalizedTitle,
        });
        setDocuments((currentDocuments) => [
          document,
          ...currentDocuments.filter((item) => item.id !== document.id),
        ]);
        setSelectedDocumentId(document.id);
        return true;
      } catch (requestError) {
        onError(getErrorMessage(requestError, fallbackError));
        return false;
      } finally {
        setIsCreating(false);
      }
    },
    [defaultTitle, fallbackError, onError],
  );

  const renameDocument = useCallback(
    async (document: CollabDocument, title: string) => {
      const normalizedTitle = title.trim();

      if (!normalizedTitle || normalizedTitle === document.title) {
        return true;
      }

      setIsRenaming(true);
      setDocuments((currentDocuments) =>
        currentDocuments.map((item) =>
          item.id === document.id ? { ...item, title: normalizedTitle } : item,
        ),
      );

      try {
        const nextDocument = await updateCollabDocument(document.id, {
          title: normalizedTitle,
        });
        setDocuments((currentDocuments) =>
          currentDocuments.map((item) =>
            item.id === nextDocument.id ? nextDocument : item,
          ),
        );
        return true;
      } catch (requestError) {
        setDocuments((currentDocuments) =>
          currentDocuments.map((item) =>
            item.id === document.id ? document : item,
          ),
        );
        onError(getErrorMessage(requestError, fallbackError));
        return false;
      } finally {
        setIsRenaming(false);
      }
    },
    [fallbackError, onError],
  );

  return {
    createDocument,
    documents,
    error,
    isCreating,
    isLoading,
    isRenaming,
    loadDocuments,
    renameDocument,
    selectDocument: setSelectedDocumentId,
    selectedDocument,
    selectedDocumentId,
  };
}

function getErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

export function useCollabConnection({
  document,
  userName,
}: UseCollabConnectionOptions) {
  const [status, setStatus] = useState<CollabConnectionStatus>("idle");
  const [role, setRole] = useState<CollabDocumentRole | null>(null);
  const [ydoc, setYdoc] = useState<Y.Doc | null>(null);
  const [provider, setProvider] = useState<ReturnType<
    typeof createCollabProvider
  > | null>(null);
  const [onlineUsers, setOnlineUsers] = useState<CollabUserProfile[]>([]);
  const [unsyncedChanges, setUnsyncedChanges] = useState(0);
  const [error, setError] = useState("");
  const [user, setUser] = useState<CollabUserProfile | null>(null);

  useEffect(() => {
    if (!document) {
      setStatus("idle");
      setRole(null);
      setYdoc(null);
      setProvider(null);
      setOnlineUsers([]);
      setUnsyncedChanges(0);
      setError("");
      setUser(null);
      return;
    }

    const targetDocument = document;
    let cancelled = false;
    let activeProvider: ReturnType<typeof createCollabProvider> | null = null;
    let activeYdoc: Y.Doc | null = null;

    async function connect() {
      setStatus("connecting");
      setRole(null);
      setYdoc(null);
      setProvider(null);
      setOnlineUsers([]);
      setUnsyncedChanges(0);
      setError("");

      try {
        const session = await createCollabDocumentSession(targetDocument.id);
        const nextYdoc = new Y.Doc();
        const nextUser = getCollabUserProfile(
          userName,
          session.role,
          targetDocument.id,
        );

        if (cancelled) {
          nextYdoc.destroy();
          return;
        }

        activeYdoc = nextYdoc;
        activeProvider = createCollabProvider({
          documentId: targetDocument.id,
          onAuthenticationFailed: (reason) => {
            setStatus("unauthorized");
            setError(reason);
          },
          onStatusChange: setStatus,
          onSyncedChange: (isSynced) => {
            if (isSynced) {
              setStatus("synced");
            }
          },
          onUnsyncedChanges: setUnsyncedChanges,
          onUsersChange: setOnlineUsers,
          session,
          user: nextUser,
          ydoc: nextYdoc,
        });

        setRole(session.role);
        setUser(nextUser);
        setYdoc(nextYdoc);
        setProvider(activeProvider);
      } catch (requestError) {
        if (cancelled) {
          return;
        }

        const message =
          requestError instanceof Error
            ? requestError.message
            : "Unable to start collaboration session";
        setStatus("error");
        setError(message);
        toast.error(message);
      }
    }

    void connect();

    return () => {
      cancelled = true;
      activeProvider?.destroy();
      activeYdoc?.destroy();
    };
  }, [document?.id, userName]);

  return {
    canEdit: role === "editor",
    error,
    isConnecting: status === "connecting",
    onlineUsers,
    provider,
    role,
    status,
    unsyncedChanges,
    user,
    ydoc,
  };
}

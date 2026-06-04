import { Hocuspocus } from "@hocuspocus/server";

import type { CollabAuthenticator } from "../auth/session-token.js";
import type { CollabConfig } from "../config.js";
import type { DocumentPersistence } from "../persistence/document-persistence.js";

export interface CollabConnectionContext {
  documentId?: string;
  userId?: string;
  role?: "editor" | "viewer";
}

export function createCollabServer(
  config: CollabConfig,
  authenticator: CollabAuthenticator,
  persistence: DocumentPersistence,
): Hocuspocus<CollabConnectionContext> {
  return new Hocuspocus<CollabConnectionContext>({
    name: "mpp-collab-service",
    timeout: config.COLLAB_HEARTBEAT_SECONDS * 1_000,
    debounce: config.COLLAB_UPDATE_FLUSH_MS,
    maxDebounce: config.COLLAB_UPDATE_FLUSH_MAX_MS,
    quiet: true,
    async onConnect({ context, requestParameters, connectionConfig }) {
      const token = requestParameters.get("token");
      if (!token) {
        return;
      }

      const session = await authenticator.verify(token, context.documentId);
      context.userId = session.userId;
      context.role = session.role;
      connectionConfig.isAuthenticated = true;
      connectionConfig.readOnly = session.role === "viewer";
    },
    async onAuthenticate({ context, token, connectionConfig }) {
      const session = await authenticator.verify(token, context.documentId);
      context.userId = session.userId;
      context.role = session.role;
      connectionConfig.isAuthenticated = true;
      connectionConfig.readOnly = session.role === "viewer";
    },
    async onLoadDocument({ context, document, documentName }) {
      await persistence.load(context.documentId ?? documentName, document);
    },
    async onChange({ context, documentName, update }) {
      await persistence.appendUpdate(
        context.documentId ?? documentName,
        update,
        context.userId,
      );
    },
    async onStoreDocument({ lastContext, document, documentName }) {
      await persistence.store(lastContext.documentId ?? documentName, document);
    },
  });
}

export async function closeCollabServer(
  collabServer: Hocuspocus<CollabConnectionContext>,
): Promise<void> {
  const pendingStores = flushPendingStoreHooks(collabServer);

  collabServer.closeConnections();

  await Promise.all(pendingStores);
  await Promise.all(
    Array.from(collabServer.documents.values(), (document) =>
      document.saveMutex.waitForUnlock(),
    ),
  );
  await waitForPendingDocumentUnloads(collabServer);
}

function flushPendingStoreHooks(
  collabServer: Hocuspocus<CollabConnectionContext>,
): Promise<unknown>[] {
  return Array.from(collabServer.documents.values()).flatMap((document) => {
    const debounceId = `onStoreDocument-${document.name}`;
    if (document.isLoading || !collabServer.debouncer.isDebounced(debounceId)) {
      return [];
    }

    return [Promise.resolve(collabServer.debouncer.executeNow(debounceId))];
  });
}

async function waitForPendingDocumentUnloads(
  collabServer: Hocuspocus<CollabConnectionContext>,
): Promise<void> {
  await new Promise<void>((resolve) => {
    setTimeout(resolve, 0);
  });

  await Promise.all(collabServer.unloadingDocuments.values());
}

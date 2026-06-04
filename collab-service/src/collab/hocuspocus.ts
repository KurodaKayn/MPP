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
    async onStoreDocument({ lastContext, document, documentName }) {
      await persistence.store(lastContext.documentId ?? documentName, document);
    },
  });
}

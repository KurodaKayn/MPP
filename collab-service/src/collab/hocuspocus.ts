import { Hocuspocus } from "@hocuspocus/server";

import type { CollabConfig } from "../config.js";

export interface CollabConnectionContext {
  documentId?: string;
}

export function createCollabServer(
  config: CollabConfig,
): Hocuspocus<CollabConnectionContext> {
  return new Hocuspocus<CollabConnectionContext>({
    name: "mpp-collab-service",
    timeout: config.COLLAB_HEARTBEAT_SECONDS * 1_000,
    debounce: config.COLLAB_UPDATE_FLUSH_MS,
    maxDebounce: config.COLLAB_UPDATE_FLUSH_MAX_MS,
    quiet: true,
  });
}

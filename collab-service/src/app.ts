import websocket from "@fastify/websocket";
import Fastify from "fastify";

import { createCollabAuthenticator } from "./auth/session-token.js";
import { createCollabServer } from "./collab/hocuspocus.js";
import { loadConfig } from "./config.js";
import { createMetrics } from "./metrics.js";

import type { WebSocketLike } from "@hocuspocus/server";
import type { FastifyInstance } from "fastify";
import type { CollabConfig } from "./config.js";

interface CollabDocumentParams {
  documentId: string;
}

interface CollabDocumentQuery {
  token?: string;
}

export async function buildApp(
  config: CollabConfig = loadConfig(),
): Promise<FastifyInstance> {
  const app = Fastify({
    logger: {
      level: config.LOG_LEVEL,
    },
  });
  const metrics = createMetrics();
  const authenticator = createCollabAuthenticator(config);
  const collabServer = createCollabServer(config, authenticator);

  await app.register(websocket);

  app.get("/health", async () => ({
    status: "healthy",
  }));

  app.get("/ready", async () => ({
    status: "ready",
    dependencies: {
      database_configured: Boolean(config.DATABASE_URL),
      redis_addr: config.REDIS_ADDR,
      token_secret_configured: Boolean(config.COLLAB_TOKEN_SECRET),
    },
  }));

  app.get("/metrics", async (_request, reply) => {
    reply.header("Content-Type", metrics.registry.contentType);
    return metrics.registry.metrics();
  });

  app.get<{ Params: CollabDocumentParams; Querystring: CollabDocumentQuery }>(
    config.COLLAB_WS_PATH,
    { websocket: true },
    (socket, request) => {
      if (!request.query.token) {
        socket.close(1008, "collab session token required");
        return;
      }

      collabServer.handleConnection(
        socket as unknown as WebSocketLike,
        request.raw as unknown as Request,
        {
          documentId: request.params.documentId,
        },
      );
    },
  );

  app.addHook("onClose", async () => {
    collabServer.flushPendingStores();
    collabServer.closeConnections();
  });

  return app;
}

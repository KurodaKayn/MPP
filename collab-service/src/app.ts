import websocket from "@fastify/websocket";
import Fastify from "fastify";
import { z } from "zod";

import { isInternalTokenAuthorized } from "./auth/internal-token.js";
import { createCollabAuthenticator } from "./auth/session-token.js";
import { closeCollabServer, createCollabServer } from "./collab/hocuspocus.js";
import { loadConfig } from "./config.js";
import { createMetrics } from "./metrics.js";
import { createPostgresDocumentPersistence } from "./persistence/document-persistence.js";

import type { WebSocketLike } from "@hocuspocus/server";
import type { FastifyInstance } from "fastify";
import type { CollabConfig } from "./config.js";
import type { DocumentPersistence } from "./persistence/document-persistence.js";

interface CollabDocumentParams {
  documentId: string;
}

const DocumentIdSchema = z.string().uuid();

export interface BuildAppOptions {
  persistence?: DocumentPersistence;
}

export async function buildApp(
  config: CollabConfig = loadConfig(),
  options: BuildAppOptions = {},
): Promise<FastifyInstance> {
  const app = Fastify({
    logger: {
      level: config.LOG_LEVEL,
    },
  });
  const metrics = createMetrics();
  const authenticator = createCollabAuthenticator(config);
  const persistence =
    options.persistence ?? createPostgresDocumentPersistence(config);
  const collabServer = createCollabServer(config, authenticator, persistence);

  await app.register(websocket);

  app.get("/health", async () => ({
    status: "healthy",
  }));

  app.get("/ready", async (_request, reply) => {
    try {
      await persistence.ping();
      return {
        status: "ready",
        dependencies: {
          database: "ready",
          redis_addr: config.REDIS_ADDR,
          token_secret_configured: Boolean(config.COLLAB_TOKEN_SECRET),
        },
      };
    } catch {
      reply.code(503);
      return {
        status: "not_ready",
        dependency: "database",
      };
    }
  });

  app.get("/metrics", async (_request, reply) => {
    reply.header("Content-Type", metrics.registry.contentType);
    return metrics.registry.metrics();
  });

  app.post<{ Params: CollabDocumentParams }>(
    "/internal/collab/documents/:documentId/project-state",
    async (request, reply) => {
      if (
        !isInternalTokenAuthorized(
          request.headers.authorization,
          config.COLLAB_TOKEN_SECRET,
        )
      ) {
        return reply.code(401).send({ error: "unauthorized" });
      }

      const documentId = DocumentIdSchema.safeParse(request.params.documentId);
      if (!documentId.success) {
        return reply.code(400).send({ error: "invalid document id" });
      }

      try {
        const initialized = await persistence.initializeProjectDocument(
          documentId.data,
        );
        if (!initialized) {
          return reply.code(404).send({ error: "project document not found" });
        }
        return reply.code(204).send();
      } catch (error) {
        request.log.error(
          { documentId: documentId.data, error },
          "failed to initialize project collaboration state",
        );
        return reply
          .code(503)
          .send({ error: "project document initialization failed" });
      }
    },
  );

  app.get<{ Params: CollabDocumentParams }>(
    config.COLLAB_WS_PATH,
    { websocket: true },
    (socket, request) => {
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
    await closeCollabServer(collabServer);
    await persistence.close();
  });

  return app;
}

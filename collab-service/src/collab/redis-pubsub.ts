import { createClient } from "redis";
import { applyUpdate } from "yjs";
import { createHash, randomUUID } from "node:crypto";

import type { Document } from "@hocuspocus/server";
import type { RedisClientType } from "redis";
import type { CollabConfig } from "../config.js";

interface CollabUpdateEnvelope {
  actorUserId?: string;
  documentId: string;
  instanceId: string;
  update: string;
  updateHash: string;
  updateId: string;
}

interface RedisPubSubLogger {
  error(message?: unknown, ...optionalParams: unknown[]): void;
  info?(message?: unknown, ...optionalParams: unknown[]): void;
  warn?(message?: unknown, ...optionalParams: unknown[]): void;
}

export interface CollabRedisPubSub {
  close(): Promise<void>;
  isRemoteUpdate(update: Uint8Array): boolean;
  publishUpdate(
    documentId: string,
    update: Uint8Array,
    actorUserId?: string,
  ): Promise<void>;
  start(documents: Map<string, Document>): Promise<void>;
}

type RedisClient = RedisClientType;

export class RedisCollabPubSub implements CollabRedisPubSub {
  private documents?: Map<string, Document>;
  private readonly remoteUpdateHashes = new Set<string>();
  private readonly instanceId = randomUUID();

  constructor(
    private readonly publisher: RedisClient,
    private readonly subscriber: RedisClient,
    private readonly channelPrefix: string,
    private readonly logger: RedisPubSubLogger = console,
  ) {}

  async start(documents: Map<string, Document>): Promise<void> {
    this.documents = documents;
    await this.publisher.connect();
    await this.subscriber.connect();
    await this.subscriber.pSubscribe(`${this.channelPrefix}:*`, (message) => {
      this.handleMessage(message);
    });
  }

  async publishUpdate(
    documentId: string,
    update: Uint8Array,
    actorUserId?: string,
  ): Promise<void> {
    const updateHash = hashUpdate(update);
    const envelope: CollabUpdateEnvelope = {
      actorUserId,
      documentId,
      instanceId: this.instanceId,
      update: Buffer.from(update).toString("base64"),
      updateHash,
      updateId: randomUUID(),
    };
    await this.publisher.publish(
      this.channel(documentId),
      JSON.stringify(envelope),
    );
  }

  isRemoteUpdate(update: Uint8Array): boolean {
    const updateHash = hashUpdate(update);
    if (!this.remoteUpdateHashes.has(updateHash)) {
      return false;
    }
    this.remoteUpdateHashes.delete(updateHash);
    return true;
  }

  async close(): Promise<void> {
    await Promise.allSettled([this.subscriber.quit(), this.publisher.quit()]);
  }

  private handleMessage(message: string): void {
    const envelope = parseEnvelope(message);
    if (!envelope || envelope.instanceId === this.instanceId) {
      return;
    }

    const document = this.documents?.get(envelope.documentId);
    if (!document) {
      return;
    }

    const update = Buffer.from(envelope.update, "base64");
    if (hashUpdate(update) !== envelope.updateHash) {
      this.logger.warn?.("discarding collab redis update with invalid hash", {
        documentId: envelope.documentId,
      });
      return;
    }

    this.remoteUpdateHashes.add(envelope.updateHash);
    try {
      applyUpdate(document, new Uint8Array(update), "redis-pubsub");
    } catch (error) {
      this.remoteUpdateHashes.delete(envelope.updateHash);
      this.logger.error("failed to apply collab redis update", {
        documentId: envelope.documentId,
        error,
      });
    }
  }

  private channel(documentId: string): string {
    return `${this.channelPrefix}:${documentId}`;
  }
}

export function createRedisCollabPubSub(
  config: CollabConfig,
  logger: RedisPubSubLogger = console,
): CollabRedisPubSub | undefined {
  if (!config.COLLAB_REDIS_SYNC_ENABLED) {
    return undefined;
  }
  const url = redisUrlFromConfig(config);
  const socketOptions = {
    reconnectStrategy(retries: number) {
      return Math.min(100 + retries * 100, 2_000);
    },
  };
  return new RedisCollabPubSub(
    createClient({ database: config.REDIS_DB, password: config.REDIS_PASSWORD || undefined, socket: socketOptions, url }),
    createClient({ database: config.REDIS_DB, password: config.REDIS_PASSWORD || undefined, socket: socketOptions, url }),
    config.COLLAB_REDIS_CHANNEL_PREFIX,
    logger,
  );
}

function redisUrlFromConfig(config: CollabConfig): string {
  const raw = config.REDIS_ADDR.trim();
  if (raw.startsWith("redis://") || raw.startsWith("rediss://")) {
    return raw;
  }
  return `redis://${raw}`;
}

function parseEnvelope(message: string): CollabUpdateEnvelope | undefined {
  try {
    const parsed = JSON.parse(message) as Partial<CollabUpdateEnvelope>;
    if (
      typeof parsed.documentId !== "string" ||
      typeof parsed.instanceId !== "string" ||
      typeof parsed.update !== "string" ||
      typeof parsed.updateHash !== "string" ||
      typeof parsed.updateId !== "string"
    ) {
      return undefined;
    }
    return parsed as CollabUpdateEnvelope;
  } catch {
    return undefined;
  }
}

function hashUpdate(update: Uint8Array): string {
  return createHash("sha256").update(update).digest("hex");
}

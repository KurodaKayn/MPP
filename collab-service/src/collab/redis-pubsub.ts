import { createClient, createSentinel } from "redis";
import { applyUpdate } from "yjs";
import { createHash, randomUUID } from "node:crypto";

import type { Document } from "@hocuspocus/server";
import type { RedisClientOptions } from "redis";
import type { CollabConfig } from "../config.js";

type RedisSentinelOptions = Parameters<typeof createSentinel>[0];

const remoteUpdateHashTTLMS = 60_000;
const remoteUpdateHashMaxEntries = 4096;
const redisConnectTimeoutMS = 1_000;
const redisSocketTimeoutMS = 1_000;
const redisPingIntervalMS = 15_000;
const redisCommandQueueMaxLength = 256;
const redisMaxReconnectDelayMS = 2_000;
const redisMaxReconnectRetries = 3;
const redisReconnectJitterMS = 50;

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
  isReady(): boolean;
  isRemoteUpdate(update: Uint8Array): boolean;
  publishUpdate(
    documentId: string,
    update: Uint8Array,
    actorUserId?: string,
  ): Promise<void>;
  start(documents: Map<string, Document>): Promise<void>;
}

interface RedisClient {
  close?(): Promise<unknown>;
  connect(): Promise<unknown>;
  isReady?: boolean;
  on?(event: string, listener: (...args: unknown[]) => void): unknown;
  pSubscribe(
    pattern: string,
    listener: (message: string) => void,
  ): Promise<unknown>;
  publish(channel: string, message: string): Promise<unknown>;
  quit?(): Promise<unknown>;
}

export class RedisCollabPubSub implements CollabRedisPubSub {
  private documents?: Map<string, Document>;
  private connected = false;
  private readonly remoteUpdateHashes = new Map<string, number>();
  private readonly instanceId = randomUUID();

  constructor(
    private readonly publisher: RedisClient,
    private readonly subscriber: RedisClient,
    private readonly channelPrefix: string,
    private readonly logger: RedisPubSubLogger = console,
  ) {}

  async start(documents: Map<string, Document>): Promise<void> {
    this.documents = documents;
    attachRedisClientLogging(this.publisher, "publisher", this.logger);
    attachRedisClientLogging(this.subscriber, "subscriber", this.logger);
    await this.publisher.connect();
    await this.subscriber.connect();
    await this.subscriber.pSubscribe(`${this.channelPrefix}:*`, (message) => {
      this.handleMessage(message);
    });
    this.connected = true;
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
    const now = Date.now();
    this.pruneRemoteUpdateHashes(now);
    const updateHash = hashUpdate(update);
    if (!this.remoteUpdateHashes.has(updateHash)) {
      return false;
    }
    this.remoteUpdateHashes.delete(updateHash);
    return true;
  }

  async close(): Promise<void> {
    this.connected = false;
    await Promise.allSettled([
      closeRedisClient(this.subscriber),
      closeRedisClient(this.publisher),
    ]);
  }

  isReady(): boolean {
    return (
      this.connected &&
      this.publisher.isReady !== false &&
      this.subscriber.isReady !== false
    );
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

    this.rememberRemoteUpdateHash(envelope.updateHash);
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

  private rememberRemoteUpdateHash(updateHash: string): void {
    const now = Date.now();
    this.pruneRemoteUpdateHashes(now);
    this.remoteUpdateHashes.set(updateHash, now + remoteUpdateHashTTLMS);
    while (this.remoteUpdateHashes.size > remoteUpdateHashMaxEntries) {
      const oldest = this.remoteUpdateHashes.keys().next().value;
      if (!oldest) {
        return;
      }
      this.remoteUpdateHashes.delete(oldest);
    }
  }

  private pruneRemoteUpdateHashes(now: number): void {
    for (const [updateHash, expiresAt] of this.remoteUpdateHashes) {
      if (expiresAt <= now) {
        this.remoteUpdateHashes.delete(updateHash);
      }
    }
  }
}

export function createRedisCollabPubSub(
  config: CollabConfig,
  logger: RedisPubSubLogger = console,
): CollabRedisPubSub | undefined {
  if (!config.COLLAB_REDIS_SYNC_ENABLED) {
    return undefined;
  }
  return new RedisCollabPubSub(
    createRedisClientFromConfig(config),
    createRedisClientFromConfig(config),
    config.COLLAB_REDIS_CHANNEL_PREFIX,
    logger,
  );
}

export function createRedisClientFromConfig(config: CollabConfig): RedisClient {
  if (config.REDIS_ENDPOINT_MODE === "sentinel") {
    return createSentinel(
      redisSentinelOptionsFromConfig(config),
    ) as unknown as RedisClient;
  }
  return createClient(
    redisClientOptionsFromConfig(config),
  ) as unknown as RedisClient;
}

export function redisClientOptionsFromConfig(
  config: CollabConfig,
): RedisClientOptions {
  return {
    commandsQueueMaxLength: redisCommandQueueMaxLength,
    disableOfflineQueue: true,
    database: config.REDIS_DB,
    pingInterval: redisPingIntervalMS,
    password: config.REDIS_PASSWORD || undefined,
    socket: {
      connectTimeout: redisConnectTimeoutMS,
      socketTimeout: redisSocketTimeoutMS,
      reconnectStrategy: redisReconnectStrategy,
    },
    url: redisUrlFromConfig(config),
  };
}

export function redisSentinelOptionsFromConfig(
  config: CollabConfig,
): RedisSentinelOptions {
  const socket = {
    ...(config.REDIS_TLS ? { tls: true } : {}),
    connectTimeout: redisConnectTimeoutMS,
    socketTimeout: redisSocketTimeoutMS,
    reconnectStrategy: redisReconnectStrategy,
  };
  return {
    name: config.REDIS_SENTINEL_MASTER_NAME,
    passthroughClientErrorEvents: true,
    sentinelRootNodes: redisSentinelRootNodesFromConfig(config),
    sentinelClientOptions: {
      commandsQueueMaxLength: redisCommandQueueMaxLength,
      disableOfflineQueue: true,
      pingInterval: redisPingIntervalMS,
      socket,
    },
    nodeClientOptions: {
      commandsQueueMaxLength: redisCommandQueueMaxLength,
      disableOfflineQueue: true,
      database: config.REDIS_DB,
      pingInterval: redisPingIntervalMS,
      password: config.REDIS_PASSWORD || undefined,
      socket,
    },
  };
}

export function redisSentinelRootNodesFromConfig(
  config: CollabConfig,
): Array<{ host: string; port: number }> {
  return config.REDIS_SENTINEL_ADDRS.split(",")
    .map((value) => value.trim())
    .filter(Boolean)
    .map(parseHostPort);
}

function redisUrlFromConfig(config: CollabConfig): string {
  const raw = config.REDIS_ADDR.trim();
  if (raw.startsWith("rediss://")) {
    return raw;
  }
  if (raw.startsWith("redis://")) {
    if (!config.REDIS_TLS) {
      return raw;
    }
    return `rediss://${raw.slice("redis://".length)}`;
  }
  const scheme = config.REDIS_TLS ? "rediss" : "redis";
  return `${scheme}://${raw}`;
}

function parseHostPort(value: string): { host: string; port: number } {
  const separator = value.lastIndexOf(":");
  const host = value.slice(0, separator).trim();
  const rawPort = value.slice(separator + 1).trim();
  const port = Number(rawPort);
  if (
    separator <= 0 ||
    host === "" ||
    !Number.isInteger(port) ||
    port < 1 ||
    port > 65_535
  ) {
    throw new Error(`invalid REDIS_SENTINEL_ADDRS entry ${value}`);
  }
  return { host, port };
}

function closeRedisClient(client: RedisClient): Promise<unknown> {
  if (client.close) {
    return client.close();
  }
  return client.quit?.() ?? Promise.resolve();
}

function redisReconnectStrategy(retries: number): number | Error {
  if (retries > redisMaxReconnectRetries) {
    return new Error("collab redis reconnect retry limit exceeded");
  }
  const backoff = Math.min(100 + retries * 100, redisMaxReconnectDelayMS);
  return backoff + Math.floor(Math.random() * redisReconnectJitterMS);
}

function attachRedisClientLogging(
  client: RedisClient,
  role: string,
  logger: RedisPubSubLogger,
): void {
  client.on?.("error", (error) => {
    logger.error("collab redis client error", { role, error });
  });
  client.on?.("reconnecting", () => {
    logger.warn?.("collab redis client reconnecting", { role });
  });
  client.on?.("ready", () => {
    logger.info?.("collab redis client ready", { role });
  });
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

import { Document } from "@hocuspocus/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { encodeStateAsUpdate } from "yjs";

import { loadConfig } from "../config.js";
import {
  redisClientOptionsFromConfig,
  redisSentinelOptionsFromConfig,
  redisSentinelRootNodesFromConfig,
  RedisCollabPubSub,
} from "./redis-pubsub.js";

type MessageHandler = (message: string) => void;

class InMemoryRedisBus {
  subscribers = new Set<MessageHandler>();

  publish(_channel: string, message: string) {
    for (const subscriber of this.subscribers) {
      subscriber(message);
    }
  }
}

class FakeRedisClient {
  isReady = true;

  constructor(private readonly bus: InMemoryRedisBus) {}

  async connect() {}

  async pSubscribe(_pattern: string, handler: MessageHandler) {
    this.bus.subscribers.add(handler);
  }

  async publish(channel: string, message: string) {
    this.bus.publish(channel, message);
  }

  async quit() {}
}

describe("RedisCollabPubSub", () => {
  let bus: InMemoryRedisBus;

  beforeEach(() => {
    bus = new InMemoryRedisBus();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("applies updates from another collab-service instance", async () => {
    const source = new Document("document-1");
    const target = new Document("document-1");
    const firstInstance = new RedisCollabPubSub(
      new FakeRedisClient(bus) as never,
      new FakeRedisClient(bus) as never,
      "mpp:test:collab",
    );
    const secondInstance = new RedisCollabPubSub(
      new FakeRedisClient(bus) as never,
      new FakeRedisClient(bus) as never,
      "mpp:test:collab",
    );
    await firstInstance.start(new Map([["document-1", source]]));
    await secondInstance.start(new Map([["document-1", target]]));

    source.getMap("content").set("title", "Synced across instances");
    await firstInstance.publishUpdate(
      "document-1",
      encodeStateAsUpdate(source),
      "user-1",
    );

    expect(target.getMap("content").get("title")).toBe(
      "Synced across instances",
    );
    const remoteUpdate = encodeStateAsUpdate(target);
    expect(secondInstance.isRemoteUpdate(remoteUpdate)).toBe(true);
    expect(secondInstance.isRemoteUpdate(remoteUpdate)).toBe(false);
  });

  it("expires unconsumed remote update markers", async () => {
    vi.useFakeTimers();
    const source = new Document("document-1");
    const target = new Document("document-1");
    const firstInstance = new RedisCollabPubSub(
      new FakeRedisClient(bus) as never,
      new FakeRedisClient(bus) as never,
      "mpp:test:collab",
    );
    const secondInstance = new RedisCollabPubSub(
      new FakeRedisClient(bus) as never,
      new FakeRedisClient(bus) as never,
      "mpp:test:collab",
    );
    await firstInstance.start(new Map([["document-1", source]]));
    await secondInstance.start(new Map([["document-1", target]]));

    source.getMap("content").set("title", "Remote update");
    await firstInstance.publishUpdate(
      "document-1",
      encodeStateAsUpdate(source),
      "user-1",
    );

    const remoteUpdate = encodeStateAsUpdate(target);
    vi.advanceTimersByTime(60_001);

    expect(secondInstance.isRemoteUpdate(remoteUpdate)).toBe(false);
  });
});

describe("redisClientOptionsFromConfig", () => {
  it("uses plaintext Redis by default", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      REDIS_ADDR: "redis.example.invalid:6379",
    });

    const options = redisClientOptionsFromConfig(config);

    expect(options.url).toBe("redis://redis.example.invalid:6379");
  });

  it("uses rediss when REDIS_TLS is enabled", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      REDIS_ADDR: "redis.example.invalid:6379",
      REDIS_TLS: "true",
    });

    const options = redisClientOptionsFromConfig(config);

    expect(options.url).toBe("rediss://redis.example.invalid:6379");
  });

  it("upgrades explicit redis URLs when REDIS_TLS is enabled", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      REDIS_ADDR: "redis://redis.example.invalid:6379",
      REDIS_TLS: "true",
    });

    const options = redisClientOptionsFromConfig(config);

    expect(options.url).toBe("rediss://redis.example.invalid:6379");
  });

  it("preserves explicit rediss URLs", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      REDIS_ADDR: "rediss://redis.example.invalid:6379",
    });

    const options = redisClientOptionsFromConfig(config);

    expect(options.url).toBe("rediss://redis.example.invalid:6379");
  });

  it("applies bounded reconnect and queue defaults", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      REDIS_ADDR: "redis.example.invalid:6379",
    });

    const options = redisClientOptionsFromConfig(config);

    expect(options.commandsQueueMaxLength).toBe(256);
    expect(options.disableOfflineQueue).toBe(true);
    expect(options.pingInterval).toBe(15_000);
    expect(options.socket).toMatchObject({
      connectTimeout: 1_000,
      socketTimeout: 1_000,
    });
    const reconnectDelay = options.socket?.reconnectStrategy?.(3);
    expect(typeof reconnectDelay).toBe("number");
    expect(reconnectDelay).toBeGreaterThanOrEqual(400);
    expect(reconnectDelay).toBeLessThan(450);
    expect(options.socket?.reconnectStrategy?.(4)).toBeInstanceOf(Error);
  });
});

describe("redisSentinelOptionsFromConfig", () => {
  it("builds sentinel options from config", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      REDIS_ENDPOINT_MODE: "sentinel",
      REDIS_SENTINEL_ADDRS:
        " redis-ha-sentinel:26379,redis-ha-sentinel-1:26379 ",
      REDIS_SENTINEL_MASTER_NAME: "mpp-redis-ha",
      REDIS_PASSWORD: "redis-secret",
      REDIS_DB: "2",
      REDIS_TLS: "true",
    });

    const options = redisSentinelOptionsFromConfig(config);

    expect(options.name).toBe("mpp-redis-ha");
    expect(options.sentinelRootNodes).toEqual([
      { host: "redis-ha-sentinel", port: 26379 },
      { host: "redis-ha-sentinel-1", port: 26379 },
    ]);
    expect(options.nodeClientOptions).toMatchObject({
      commandsQueueMaxLength: 256,
      disableOfflineQueue: true,
      database: 2,
      pingInterval: 15_000,
      password: "redis-secret",
      socket: { tls: true, connectTimeout: 1_000, socketTimeout: 1_000 },
    });
    expect(options.sentinelClientOptions).toMatchObject({
      commandsQueueMaxLength: 256,
      disableOfflineQueue: true,
      pingInterval: 15_000,
      socket: { tls: true, connectTimeout: 1_000, socketTimeout: 1_000 },
    });
    expect(options.passthroughClientErrorEvents).toBe(true);
  });

  it("rejects sentinel mode without sentinel addrs", () => {
    expect(() =>
      loadConfig({
        NODE_ENV: "test",
        REDIS_ENDPOINT_MODE: "sentinel",
      }),
    ).toThrow(/REDIS_SENTINEL_ADDRS/);
  });

  it("rejects invalid sentinel hostport entries", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      REDIS_ENDPOINT_MODE: "sentinel",
      REDIS_SENTINEL_ADDRS: "redis-ha-sentinel",
    });

    expect(() => redisSentinelRootNodesFromConfig(config)).toThrow(
      /REDIS_SENTINEL_ADDRS/,
    );
  });
});

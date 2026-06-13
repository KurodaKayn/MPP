import { Document } from "@hocuspocus/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { encodeStateAsUpdate } from "yjs";

import { loadConfig } from "../config.js";
import {
  redisClientOptionsFromConfig,
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
});

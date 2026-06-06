import { Document } from "@hocuspocus/server";
import { beforeEach, describe, expect, it } from "vitest";
import { encodeStateAsUpdate } from "yjs";

import { RedisCollabPubSub } from "./redis-pubsub.js";

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
});

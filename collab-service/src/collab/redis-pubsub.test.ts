import { Document } from "@hocuspocus/server";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { encodeStateAsUpdate } from "yjs";

import { loadConfig } from "../config.js";
import {
  redisClusterOptionsFromConfig,
  redisClusterRootNodesFromConfig,
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

  it("applies TLS CA and server name options", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      REDIS_ADDR: "redis.example.invalid:6379",
      REDIS_TLS: "true",
      REDIS_TLS_CA_CERT: testRedisCACertPEM,
      REDIS_TLS_SERVER_NAME: "redis.internal.example",
    });

    const options = redisClientOptionsFromConfig(config);

    expect(options.socket).toMatchObject({
      ca: testRedisCACertPEM,
      servername: "redis.internal.example",
      tls: true,
    });
  });

  it("reads TLS CA options from a file", () => {
    const caFile = join(mkdtempSync(join(tmpdir(), "mpp-redis-ca-")), "ca.pem");
    writeFileSync(caFile, testRedisCACertPEM);
    const config = loadConfig({
      NODE_ENV: "test",
      REDIS_ADDR: "redis.example.invalid:6379",
      REDIS_TLS: "true",
      REDIS_TLS_CA_FILE: caFile,
    });

    const options = redisClientOptionsFromConfig(config);

    expect(options.socket).toMatchObject({
      ca: testRedisCACertPEM,
      tls: true,
    });
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

describe("redisClusterOptionsFromConfig", () => {
  it("builds cluster options from config", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      REDIS_ENDPOINT_MODE: "cluster",
      REDIS_ADDR: " redis-cluster-0:6379,redis-cluster-1:6379 ",
      REDIS_PASSWORD: "redis-secret",
      REDIS_TLS: "true",
      REDIS_TLS_CA_CERT: testRedisCACertPEM,
      REDIS_TLS_SERVER_NAME: "redis.internal.example",
    });

    const options = redisClusterOptionsFromConfig(config);

    expect(options.maxCommandRedirections).toBe(8);
    expect(options.rootNodes).toEqual([
      { url: "rediss://redis-cluster-0:6379" },
      { url: "rediss://redis-cluster-1:6379" },
    ]);
    expect(options.defaults).toMatchObject({
      commandsQueueMaxLength: 256,
      disableOfflineQueue: true,
      database: 0,
      pingInterval: 15_000,
      password: "redis-secret",
      socket: {
        ca: testRedisCACertPEM,
        connectTimeout: 1_000,
        servername: "redis.internal.example",
        socketTimeout: 1_000,
        tls: true,
      },
    });
  });

  it("rejects cluster mode with non-zero db", () => {
    expect(() =>
      loadConfig({
        NODE_ENV: "test",
        REDIS_ENDPOINT_MODE: "cluster",
        REDIS_ADDR: "redis-cluster-0:6379",
        REDIS_DB: "1",
      }),
    ).toThrow(/REDIS_DB/);
  });

  it("parses cluster seed nodes", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      REDIS_ENDPOINT_MODE: "cluster",
      REDIS_ADDR: "redis-cluster-0:6379,redis-cluster-1:6379",
    });

    expect(redisClusterRootNodesFromConfig(config)).toEqual([
      { url: "redis://redis-cluster-0:6379" },
      { url: "redis://redis-cluster-1:6379" },
    ]);
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
      REDIS_TLS_CA_CERT: testRedisCACertPEM,
      REDIS_TLS_SERVER_NAME: "redis.internal.example",
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
      socket: {
        ca: testRedisCACertPEM,
        connectTimeout: 1_000,
        servername: "redis.internal.example",
        socketTimeout: 1_000,
        tls: true,
      },
    });
    expect(options.sentinelClientOptions).toMatchObject({
      commandsQueueMaxLength: 256,
      disableOfflineQueue: true,
      pingInterval: 15_000,
      socket: {
        ca: testRedisCACertPEM,
        connectTimeout: 1_000,
        servername: "redis.internal.example",
        socketTimeout: 1_000,
        tls: true,
      },
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

const testRedisCACertPEM = `-----BEGIN CERTIFICATE-----
MIIDEzCCAfugAwIBAgIUb15xgBiiAVRKRFX/A/p9TvypqJwwDQYJKoZIhvcNAQEL
BQAwGTEXMBUGA1UEAwwObXBwLXJlZGlzLXRlc3QwHhcNMjYwNjE4MTQyOTQ5WhcN
MjcwNjE4MTQyOTQ5WjAZMRcwFQYDVQQDDA5tcHAtcmVkaXMtdGVzdDCCASIwDQYJ
KoZIhvcNAQEBBQADggEPADCCAQoCggEBANGUc9qScjxCIirs4/uUnYWd+ikt1zJW
jhhbVGcDJe+Ooo1sB3MgUd1iEQMHhcYuYYA6qhircakcIF8kqx0gn29yWfPPA2uU
eKRMLZei7irkgM0ZoARM9WnHUsaPJ36sB3iEBGCC4OYUIFj9hBfIcUCzG/zU14qN
f0mXQLeLn8i3WtT9r47HJ30GcfE/upHO0Rd+GZPMmZbJ2y+oiH4Lrx8T+vL0U3SZ
XvTEPZmM0cYU5IQgLjqxkS0NrHzjPhP6+v75YZ354XJh0aLMAxIO+E1A8b7y457R
r4M0yBBvFOZqORR7zau0IMqq9dySm2FxOYv45R9gZMIzuEqOBvHg6xsCAwEAAaNT
MFEwHQYDVR0OBBYEFPKgwwRXzUJNYbeAxzEQaWvmjQXfMB8GA1UdIwQYMBaAFPKg
wwRXzUJNYbeAxzEQaWvmjQXfMA8GA1UdEwEB/wQFMAMBAf8wDQYJKoZIhvcNAQEL
BQADggEBACX1ipm6cO0bgt3iB24CzFZ39ETCAs78UpQXll7VbkhPIJ9WoTYK11If
6mlhEOtDDcg1s1nY91wVmA5ZnLkAIY+RkBfIDREX9tzmhcROoJRJmu8LjTmW5QmF
KJV2w16drmHd7jgosOzFrqzWjatZ4DUyc9n8c4TYV0BDph6ARE0IL+9rHXA7wakG
tYGsODtHm/A35rOUUfx34E9PUIQXrm7HPIHbThi64/vJFd2dzvB/966Z2YCtkBf2
eXFaNn/Uv31V+R4jo/IoXT3Ge5aU2/HCF4GLt86Hny8lrZI/rzBtD+mvxHiPCeVH
kXlb94L5hmllJh6r7idCx5YrKWYGYCc=
-----END CERTIFICATE-----`;

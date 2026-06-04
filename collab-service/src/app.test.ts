import { describe, expect, it } from "vitest";

import { buildApp } from "./app.js";
import { loadConfig } from "./config.js";

import type { Document } from "@hocuspocus/server";
import type { DocumentPersistence } from "./persistence/document-persistence.js";

class FakeDocumentPersistence implements DocumentPersistence {
  constructor(private readonly pingError?: Error) {}

  async load(_documentId: string, _document: Document): Promise<void> {}

  async appendUpdate(
    _documentId: string,
    _update: Uint8Array,
    _actorUserId?: string,
  ): Promise<void> {}

  async store(_documentId: string, _document: Document): Promise<void> {}

  async flush(): Promise<void> {}

  async ping(): Promise<void> {
    if (this.pingError) {
      throw this.pingError;
    }
  }

  async close(): Promise<void> {}
}

function testConfig() {
  return loadConfig({
    NODE_ENV: "test",
    LOG_LEVEL: "silent",
    COLLAB_HOST: "127.0.0.1",
    COLLAB_PORT: "8090",
    COLLAB_WS_PATH: "/collab/documents/:documentId",
    BACKEND_INTERNAL_URL: "http://backend:8080",
    COLLAB_TOKEN_SECRET: "collab-secret",
  });
}

describe("collab-service app", () => {
  it("serves health and readiness probes", async () => {
    const app = await buildApp(testConfig(), {
      persistence: new FakeDocumentPersistence(),
    });

    const health = await app.inject("/health");
    const ready = await app.inject("/ready");

    expect(health.statusCode).toBe(200);
    expect(health.json()).toEqual({ status: "healthy" });
    expect(ready.statusCode).toBe(200);
    expect(ready.json()).toMatchObject({
      status: "ready",
      dependencies: {
        database: "ready",
        redis_addr: "redis:6379",
        token_secret_configured: true,
      },
    });

    await app.close();
  });

  it("serves prometheus metrics", async () => {
    const app = await buildApp(testConfig(), {
      persistence: new FakeDocumentPersistence(),
    });

    const response = await app.inject("/metrics");

    expect(response.statusCode).toBe(200);
    expect(response.headers["content-type"]).toContain("text/plain");
    expect(response.body).toContain("collab_service_info");

    await app.close();
  });

  it("reports not ready when postgres is unavailable", async () => {
    const app = await buildApp(testConfig(), {
      persistence: new FakeDocumentPersistence(
        new Error("database unavailable"),
      ),
    });

    const response = await app.inject("/ready");

    expect(response.statusCode).toBe(503);
    expect(response.json()).toEqual({
      status: "not_ready",
      dependency: "database",
    });

    await app.close();
  });
});

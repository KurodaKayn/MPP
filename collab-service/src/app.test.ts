import { describe, expect, it } from "vitest";

import { buildApp } from "./app.js";
import { loadConfig } from "./config.js";

import type { Document } from "@hocuspocus/server";
import type { DocumentPersistence } from "./persistence/document-persistence.js";

class FakeDocumentPersistence implements DocumentPersistence {
  initializedDocumentIds: string[] = [];
  initializeProjectDocumentError?: Error;
  initializeProjectDocumentResult = true;
  syncedSourceDocumentIds: string[] = [];
  syncProjectSourceContentError?: Error;
  syncProjectSourceContentResult = true;

  constructor(private readonly pingError?: Error) {}

  async load(_documentId: string, _document: Document): Promise<void> {}

  async initializeProjectDocument(documentId: string): Promise<boolean> {
    this.initializedDocumentIds.push(documentId);
    if (this.initializeProjectDocumentError) {
      throw this.initializeProjectDocumentError;
    }
    return this.initializeProjectDocumentResult;
  }

  async syncProjectSourceContent(documentId: string): Promise<boolean> {
    this.syncedSourceDocumentIds.push(documentId);
    if (this.syncProjectSourceContentError) {
      throw this.syncProjectSourceContentError;
    }
    return this.syncProjectSourceContentResult;
  }

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
    COLLAB_REDIS_SYNC_ENABLED: "false",
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
    expect(response.body).toContain("collab_active_connections");
    expect(response.body).toContain("collab_active_documents");
    expect(response.body).toContain("collab_auth_denials_total");
    expect(response.body).toContain("collab_update_flush_latency_seconds");

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

  it("initializes project collaboration state for authorized backend requests", async () => {
    const persistence = new FakeDocumentPersistence();
    const app = await buildApp(testConfig(), { persistence });
    const documentId = "11111111-1111-4111-8111-111111111111";

    const response = await app.inject({
      method: "POST",
      url: `/internal/collab/documents/${documentId}/project-state`,
      headers: {
        authorization: "Bearer collab-secret",
      },
    });

    expect(response.statusCode).toBe(204);
    expect(persistence.initializedDocumentIds).toEqual([documentId]);

    await app.close();
  });

  it("rejects unauthorized project collaboration initialization", async () => {
    const persistence = new FakeDocumentPersistence();
    const app = await buildApp(testConfig(), { persistence });

    const response = await app.inject({
      method: "POST",
      url: "/internal/collab/documents/11111111-1111-4111-8111-111111111111/project-state",
    });

    expect(response.statusCode).toBe(401);
    expect(persistence.initializedDocumentIds).toEqual([]);

    await app.close();
  });

  it("rejects invalid project collaboration document ids", async () => {
    const persistence = new FakeDocumentPersistence();
    const app = await buildApp(testConfig(), { persistence });

    const response = await app.inject({
      method: "POST",
      url: "/internal/collab/documents/not-a-uuid/project-state",
      headers: {
        authorization: "Bearer collab-secret",
      },
    });

    expect(response.statusCode).toBe(400);
    expect(persistence.initializedDocumentIds).toEqual([]);

    await app.close();
  });

  it("returns not found when the collaboration document is not linked to a project", async () => {
    const persistence = new FakeDocumentPersistence();
    persistence.initializeProjectDocumentResult = false;
    const app = await buildApp(testConfig(), { persistence });

    const response = await app.inject({
      method: "POST",
      url: "/internal/collab/documents/11111111-1111-4111-8111-111111111111/project-state",
      headers: {
        authorization: "Bearer collab-secret",
      },
    });

    expect(response.statusCode).toBe(404);

    await app.close();
  });

  it("returns service unavailable when project collaboration initialization fails", async () => {
    const persistence = new FakeDocumentPersistence();
    persistence.initializeProjectDocumentError = new Error(
      "database unavailable",
    );
    const app = await buildApp(testConfig(), { persistence });

    const response = await app.inject({
      method: "POST",
      url: "/internal/collab/documents/11111111-1111-4111-8111-111111111111/project-state",
      headers: {
        authorization: "Bearer collab-secret",
      },
    });

    expect(response.statusCode).toBe(503);

    await app.close();
  });

  it("syncs project source content for authorized backend requests", async () => {
    const persistence = new FakeDocumentPersistence();
    const app = await buildApp(testConfig(), { persistence });
    const documentId = "11111111-1111-4111-8111-111111111111";

    const response = await app.inject({
      method: "POST",
      url: `/internal/collab/documents/${documentId}/project-source-content`,
      headers: {
        authorization: "Bearer collab-secret",
      },
    });

    expect(response.statusCode).toBe(204);
    expect(persistence.syncedSourceDocumentIds).toEqual([documentId]);

    await app.close();
  });

  it("returns not found when source content sync has no linked project", async () => {
    const persistence = new FakeDocumentPersistence();
    persistence.syncProjectSourceContentResult = false;
    const app = await buildApp(testConfig(), { persistence });

    const response = await app.inject({
      method: "POST",
      url: "/internal/collab/documents/11111111-1111-4111-8111-111111111111/project-source-content",
      headers: {
        authorization: "Bearer collab-secret",
      },
    });

    expect(response.statusCode).toBe(404);

    await app.close();
  });

  it("returns service unavailable when source content sync fails", async () => {
    const persistence = new FakeDocumentPersistence();
    persistence.syncProjectSourceContentError = new Error(
      "database unavailable",
    );
    const app = await buildApp(testConfig(), { persistence });

    const response = await app.inject({
      method: "POST",
      url: "/internal/collab/documents/11111111-1111-4111-8111-111111111111/project-source-content",
      headers: {
        authorization: "Bearer collab-secret",
      },
    });

    expect(response.statusCode).toBe(503);

    await app.close();
  });
});

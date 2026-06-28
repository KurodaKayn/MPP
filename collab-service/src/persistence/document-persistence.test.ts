import { Document } from "@hocuspocus/server";
import { describe, expect, it } from "vitest";
import { applyUpdate, encodeStateAsUpdate, encodeStateVector } from "yjs";

import {
  createProjectYDoc,
  projectYDocToProseMirrorJSON,
} from "../collab/project-document.js";
import { loadConfig } from "../config.js";
import {
  postgresPoolConfigFromConfig,
  PostgresDocumentPersistence,
} from "./document-persistence.js";

interface QueryCall {
  text: string;
  values?: unknown[];
}

interface QueryFailure {
  pattern: string;
  error: Error;
}

class FakeDatabase {
  calls: QueryCall[] = [];
  results: Record<string, unknown>[][] = [];
  failures: QueryFailure[] = [];

  async query<Row extends Record<string, unknown>>(
    text: string,
    values?: unknown[],
  ): Promise<{
    command: string;
    rowCount: number;
    oid: number;
    fields: never[];
    rows: Row[];
  }> {
    this.calls.push({ text, values });
    const failureIndex = this.failures.findIndex(({ pattern }) =>
      text.includes(pattern),
    );
    if (failureIndex >= 0) {
      const [{ error }] = this.failures.splice(failureIndex, 1);
      throw error;
    }

    const rows = text.trim().startsWith("SELECT")
      ? (this.results.shift() ?? [])
      : [];
    return {
      command: "SELECT",
      rowCount: rows.length,
      oid: 0,
      fields: [],
      rows: rows as Row[],
    };
  }
}

class FakeLogger {
  errors: unknown[][] = [];

  error(...values: unknown[]): void {
    this.errors.push(values);
  }
}

class FakeMetrics {
  flushDurations: number[] = [];

  recordUpdateFlush(durationSeconds: number): void {
    this.flushDurations.push(durationSeconds);
  }
}

async function waitForRetryTimer(): Promise<void> {
  await new Promise<void>((resolve) => {
    setTimeout(resolve, 20);
  });
}

async function waitForRetryTimers(): Promise<void> {
  await new Promise<void>((resolve) => {
    setTimeout(resolve, 50);
  });
}

describe("PostgresDocumentPersistence", () => {
  it("loads a stored Yjs snapshot", async () => {
    const source = new Document("source");
    source.getMap("content").set("title", "Persisted");
    const database = new FakeDatabase();
    database.results = [
      [
        {
          y_doc_state: Buffer.from(encodeStateAsUpdate(source)),
          compacted_until_seq: 0,
        },
      ],
      [],
    ];
    const persistence = new PostgresDocumentPersistence(database);
    const target = new Document("target");

    await persistence.load("11111111-1111-4111-8111-111111111111", target);

    expect(target.getMap("content").get("title")).toBe("Persisted");
    expect(database.calls[0]?.text).toContain("SELECT y_doc_state");
    expect(database.calls[0]?.text).not.toContain("ydoc_state");
    expect(database.calls[0]?.values).toEqual([
      "11111111-1111-4111-8111-111111111111",
    ]);
  });

  it("loads ordered update batches after the latest compacted snapshot", async () => {
    const snapshot = new Document("snapshot");
    snapshot.getMap("content").set("title", "Snapshot");
    const snapshotUpdate = encodeStateAsUpdate(snapshot);
    const snapshotVector = encodeStateVector(snapshot);
    const updateDoc = new Document("update");
    applyUpdate(updateDoc, snapshotUpdate);
    updateDoc.getMap("content").set("title", "Updated");
    const update = encodeStateAsUpdate(updateDoc, snapshotVector);
    const database = new FakeDatabase();
    database.results = [
      [
        {
          y_doc_state: Buffer.from(snapshotUpdate),
          compacted_until_seq: 4,
        },
      ],
      [
        {
          update_payload: Buffer.from(update),
        },
      ],
    ];
    const persistence = new PostgresDocumentPersistence(database);
    const target = new Document("target");

    await persistence.load("11111111-1111-4111-8111-111111111111", target);

    expect(target.getMap("content").get("title")).toBe("Updated");
    expect(database.calls[1]?.text).toContain(
      "FROM collab_document_update_batches",
    );
    expect(database.calls[1]?.text).toContain("ORDER BY from_seq ASC");
    expect(database.calls[1]?.values).toEqual([
      "11111111-1111-4111-8111-111111111111",
      4,
    ]);
  });

  it("initializes linked project source content as a Yjs snapshot", async () => {
    const database = new FakeDatabase();
    database.results = [
      [
        {
          workspace_id: "99999999-9999-4999-8999-999999999999",
          source_content:
            "<h2>Project heading</h2><p>Hello <strong>team</strong></p>",
          current_seq: 0,
          has_state: false,
          has_updates: false,
        },
      ],
    ];
    const persistence = new PostgresDocumentPersistence(database);

    const initialized = await persistence.initializeProjectDocument(
      "11111111-1111-4111-8111-111111111111",
    );

    expect(initialized).toBe(true);
    expect(database.calls[0]?.text).toContain("FROM projects");
    expect(database.calls[0]?.text).toContain("collab_documents.workspace_id");
    expect(database.calls[0]?.values).toEqual([
      "11111111-1111-4111-8111-111111111111",
    ]);
    const insertCall = database.calls[1];
    expect(insertCall?.text).toContain("INSERT INTO collab_document_states");
    expect(insertCall?.text).toContain("ON CONFLICT (document_id) DO NOTHING");
    expect(insertCall?.values?.[1]).toBe(
      "99999999-9999-4999-8999-999999999999",
    );
    expect(insertCall?.values?.[4]).toBe(0);

    const restored = new Document("restored");
    applyUpdate(restored, new Uint8Array(insertCall?.values?.[2] as Buffer));
    expect(projectYDocToProseMirrorJSON(restored)).toMatchObject({
      type: "doc",
      content: [
        {
          type: "heading",
          content: [{ type: "text", text: "Project heading" }],
        },
        {
          type: "paragraph",
          content: [
            { type: "text", text: "Hello " },
            { type: "text", text: "team" },
          ],
        },
      ],
    });
  });

  it("does not overwrite existing project collaboration state", async () => {
    const database = new FakeDatabase();
    database.results = [
      [
        {
          workspace_id: "99999999-9999-4999-8999-999999999999",
          source_content: "<p>Stale project content</p>",
          current_seq: 4,
          has_state: true,
          has_updates: false,
        },
      ],
    ];
    const persistence = new PostgresDocumentPersistence(database);

    const initialized = await persistence.initializeProjectDocument(
      "11111111-1111-4111-8111-111111111111",
    );

    expect(initialized).toBe(true);
    expect(database.calls).toHaveLength(1);
  });

  it("returns false when a collaboration document is not linked to a project", async () => {
    const database = new FakeDatabase();
    database.results = [[]];
    const persistence = new PostgresDocumentPersistence(database);

    const initialized = await persistence.initializeProjectDocument(
      "11111111-1111-4111-8111-111111111111",
    );

    expect(initialized).toBe(false);
    expect(database.calls).toHaveLength(1);
  });

  it("syncs linked project source content from the current Yjs state", async () => {
    const documentId = "11111111-1111-4111-8111-111111111111";
    const source = createProjectYDoc(
      "<h2>Synced heading</h2><p>Hello <strong>team</strong></p>",
    );
    const database = new FakeDatabase();
    database.results = [
      [
        {
          source_content: "<p>Stale project content</p>",
          current_seq: 4,
          has_state: true,
          has_updates: false,
        },
      ],
      [
        {
          y_doc_state: Buffer.from(encodeStateAsUpdate(source)),
          compacted_until_seq: 4,
        },
      ],
      [],
    ];
    const persistence = new PostgresDocumentPersistence(database);

    const synced = await persistence.syncProjectSourceContent(documentId);

    expect(synced).toBe(true);
    const updateCall = database.calls.find((call) =>
      call.text.includes("UPDATE projects"),
    );
    expect(updateCall?.text).toContain("source_content IS DISTINCT FROM $2");
    expect(updateCall?.values).toEqual([
      documentId,
      "<h2>Synced heading</h2><p>Hello <strong>team</strong></p>",
    ]);
    expect(database.calls.map((call) => call.text.trim())).toContain("COMMIT");

    source.destroy();
  });

  it("does not sync source content for unlinked collaboration documents", async () => {
    const database = new FakeDatabase();
    database.results = [[]];
    const persistence = new PostgresDocumentPersistence(database);

    const synced = await persistence.syncProjectSourceContent(
      "11111111-1111-4111-8111-111111111111",
    );

    expect(synced).toBe(false);
    expect(database.calls).toHaveLength(1);
    expect(
      database.calls.some((call) => call.text.includes("UPDATE projects")),
    ).toBe(false);
  });

  it("flushes pending Yjs updates as a sequenced batch", async () => {
    const database = new FakeDatabase();
    const metrics = new FakeMetrics();
    database.results = [
      [
        {
          current_seq: 7,
        },
      ],
      [
        {
          workspace_id: "99999999-9999-4999-8999-999999999999",
        },
      ],
    ];
    const persistence = new PostgresDocumentPersistence(
      database,
      10_000,
      32,
      30,
      5,
      30_000,
      console,
      metrics,
    );
    const first = new Document("first");
    first.getMap("content").set("title", "First");
    const second = new Document("second");
    second.getMap("content").set("body", "Second");

    await persistence.appendUpdate(
      "11111111-1111-4111-8111-111111111111",
      encodeStateAsUpdate(first),
      "22222222-2222-4222-8222-222222222222",
    );
    await persistence.appendUpdate(
      "11111111-1111-4111-8111-111111111111",
      encodeStateAsUpdate(second),
      "33333333-3333-4333-8333-333333333333",
    );
    await persistence.flush();

    expect(database.calls.map((call) => call.text.trim())).toEqual([
      "BEGIN",
      expect.stringContaining("SELECT current_seq"),
      expect.stringContaining("SELECT workspace_id"),
      expect.stringContaining("INSERT INTO collab_document_update_batches"),
      expect.stringContaining("UPDATE collab_documents"),
      "COMMIT",
    ]);
    const insertCall = database.calls[3];
    expect(insertCall?.values?.[0]).toBe(
      "11111111-1111-4111-8111-111111111111",
    );
    expect(insertCall?.values?.[1]).toBe(
      "99999999-9999-4999-8999-999999999999",
    );
    expect(insertCall?.values?.[2]).toBe(8);
    expect(insertCall?.values?.[3]).toBe(9);
    expect(insertCall?.values?.[5]).toBe(2);
    expect(insertCall?.values?.[7]).toBe(
      "33333333-3333-4333-8333-333333333333",
    );

    const restored = new Document("restored");
    applyUpdate(restored, new Uint8Array(insertCall?.values?.[4] as Buffer));
    expect(restored.getMap("content").get("title")).toBe("First");
    expect(restored.getMap("content").get("body")).toBe("Second");
    expect(metrics.flushDurations).toHaveLength(1);
    expect(metrics.flushDurations[0]).toBeGreaterThanOrEqual(0);
  });

  it("flushes immediately when the pending update count reaches the batch limit", async () => {
    const database = new FakeDatabase();
    database.results = [
      [
        {
          current_seq: 3,
        },
      ],
      [
        {
          workspace_id: "99999999-9999-4999-8999-999999999999",
        },
      ],
    ];
    const persistence = new PostgresDocumentPersistence(database, 10_000, 2);
    const first = new Document("first");
    first.getMap("content").set("title", "First");
    const second = new Document("second");
    second.getMap("content").set("body", "Second");

    await persistence.appendUpdate(
      "11111111-1111-4111-8111-111111111111",
      encodeStateAsUpdate(first),
    );
    expect(database.calls).toEqual([]);

    await persistence.appendUpdate(
      "11111111-1111-4111-8111-111111111111",
      encodeStateAsUpdate(second),
    );

    expect(database.calls.map((call) => call.text.trim())).toEqual([
      "BEGIN",
      expect.stringContaining("SELECT current_seq"),
      expect.stringContaining("SELECT workspace_id"),
      expect.stringContaining("INSERT INTO collab_document_update_batches"),
      expect.stringContaining("UPDATE collab_documents"),
      "COMMIT",
    ]);
    expect(database.calls[3]?.values?.[2]).toBe(4);
    expect(database.calls[3]?.values?.[3]).toBe(5);
  });

  it("retries batch-limit flush failures without rejecting appendUpdate", async () => {
    const database = new FakeDatabase();
    database.results = [
      [
        {
          current_seq: 3,
        },
      ],
      [
        {
          workspace_id: "99999999-9999-4999-8999-999999999999",
        },
      ],
      [
        {
          current_seq: 3,
        },
      ],
      [
        {
          workspace_id: "99999999-9999-4999-8999-999999999999",
        },
      ],
    ];
    database.failures = [
      {
        pattern: "INSERT INTO collab_document_update_batches",
        error: new Error("database unavailable"),
      },
    ];
    const logger = new FakeLogger();
    const persistence = new PostgresDocumentPersistence(
      database,
      1,
      2,
      30,
      2,
      5,
      logger,
    );
    const first = new Document("first");
    first.getMap("content").set("title", "First");
    const second = new Document("second");
    second.getMap("content").set("body", "Second");

    await persistence.appendUpdate(
      "11111111-1111-4111-8111-111111111111",
      encodeStateAsUpdate(first),
    );
    await expect(
      persistence.appendUpdate(
        "11111111-1111-4111-8111-111111111111",
        encodeStateAsUpdate(second),
      ),
    ).resolves.toBeUndefined();
    await waitForRetryTimer();

    expect(database.calls.map((call) => call.text.trim())).toContain(
      "ROLLBACK",
    );
    expect(database.calls.map((call) => call.text.trim())).toContain("COMMIT");
    expect(logger.errors[0]?.[0]).toBe("failed to flush collab update batch");
  });

  it("continues capped retries after the configured retry attempt threshold", async () => {
    const database = new FakeDatabase();
    database.results = [
      [{ current_seq: 3 }],
      [{ workspace_id: "99999999-9999-4999-8999-999999999999" }],
      [{ current_seq: 3 }],
      [{ workspace_id: "99999999-9999-4999-8999-999999999999" }],
      [{ current_seq: 3 }],
      [{ workspace_id: "99999999-9999-4999-8999-999999999999" }],
    ];
    database.failures = [
      {
        pattern: "INSERT INTO collab_document_update_batches",
        error: new Error("database unavailable"),
      },
      {
        pattern: "INSERT INTO collab_document_update_batches",
        error: new Error("database unavailable again"),
      },
    ];
    const logger = new FakeLogger();
    const persistence = new PostgresDocumentPersistence(
      database,
      1,
      1,
      30,
      2,
      1,
      logger,
    );
    const document = new Document("document");
    document.getMap("content").set("title", "Eventually saved");

    await persistence.appendUpdate(
      "11111111-1111-4111-8111-111111111111",
      encodeStateAsUpdate(document),
    );
    await waitForRetryTimers();

    expect(database.calls.map((call) => call.text.trim())).toContain("COMMIT");
    expect(logger.errors.map(([message]) => message)).toContain(
      "collab update batch retry threshold reached; continuing capped retries",
    );
  });

  it("upserts the current Yjs snapshot", async () => {
    const database = new FakeDatabase();
    database.results = [
      [
        {
          current_seq: 12,
        },
      ],
      [
        {
          workspace_id: "99999999-9999-4999-8999-999999999999",
        },
      ],
    ];
    const persistence = new PostgresDocumentPersistence(database);
    const document = new Document("document");
    document.getMap("content").set("title", "Saved");

    await persistence.store("11111111-1111-4111-8111-111111111111", document);

    expect(database.calls[0]?.text).toBe("BEGIN");
    const call = database.calls[3];
    expect(call?.text).toContain("y_doc_state");
    expect(call?.text).not.toContain("ydoc_state");
    expect(call?.text).toContain("ON CONFLICT (document_id) DO UPDATE");
    expect(call?.values?.[0]).toBe("11111111-1111-4111-8111-111111111111");
    expect(call?.values?.[1]).toBe("99999999-9999-4999-8999-999999999999");
    expect(call?.values?.[2]).toBeInstanceOf(Buffer);
    expect(call?.values?.[3]).toBeInstanceOf(Buffer);
    const state = call?.values?.[2] as Buffer;
    expect(call?.values?.[4]).toBe(12);
    expect(call?.values?.[5]).toBe(state.length);
    expect(call?.values?.[3]).toEqual(Buffer.from(encodeStateVector(document)));
    expect(database.calls[4]?.text).toBe("COMMIT");
    expect(database.calls[5]?.text).toContain(
      "DELETE FROM collab_document_update_batches",
    );
    expect(database.calls[5]?.values).toEqual([
      "11111111-1111-4111-8111-111111111111",
      12,
      30,
    ]);
  });

  it("does not roll back a snapshot when compacted batch pruning fails", async () => {
    const database = new FakeDatabase();
    database.results = [
      [
        {
          current_seq: 12,
        },
      ],
      [
        {
          workspace_id: "99999999-9999-4999-8999-999999999999",
        },
      ],
    ];
    database.failures = [
      {
        pattern: "DELETE FROM collab_document_update_batches",
        error: new Error("lock timeout"),
      },
    ];
    const logger = new FakeLogger();
    const persistence = new PostgresDocumentPersistence(
      database,
      300,
      32,
      30,
      5,
      30_000,
      logger,
    );
    const document = new Document("document");
    document.getMap("content").set("title", "Saved");

    await expect(
      persistence.store("11111111-1111-4111-8111-111111111111", document),
    ).resolves.toBeUndefined();

    expect(database.calls.map((call) => call.text.trim())).toEqual([
      "BEGIN",
      expect.stringContaining("SELECT current_seq"),
      expect.stringContaining("SELECT workspace_id"),
      expect.stringContaining("INSERT INTO collab_document_states"),
      "COMMIT",
      expect.stringContaining("DELETE FROM collab_document_update_batches"),
    ]);
    expect(database.calls.map((call) => call.text.trim())).not.toContain(
      "ROLLBACK",
    );
    expect(logger.errors[0]?.[0]).toBe(
      "failed to prune compacted collab update batches",
    );
  });
});

describe("postgresPoolConfigFromConfig", () => {
  it("does not configure TLS by default", () => {
    const config = loadConfig({ NODE_ENV: "test" });

    const poolConfig = postgresPoolConfigFromConfig(config);

    expect(poolConfig).toMatchObject({
      host: "db",
      port: 5432,
      user: "postgres",
      password: "postgres",
      database: "poster_db",
      max: 10,
      idleTimeoutMillis: 300_000,
      maxLifetimeSeconds: 1_800,
    });
    expect(poolConfig.ssl).toBeUndefined();
  });

  it("maps postgres pool settings from environment", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      DB_MAX_OPEN_CONNS: "24",
      DB_CONN_MAX_IDLE_TIME: "90s",
      DB_CONN_MAX_LIFETIME: "1h30m",
    });

    const poolConfig = postgresPoolConfigFromConfig(config);

    expect(poolConfig).toMatchObject({
      max: 24,
      idleTimeoutMillis: 90_000,
      maxLifetimeSeconds: 5_400,
    });
  });

  it("applies pool settings when using DATABASE_URL", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      DATABASE_URL: "postgres://postgres:postgres@db:5432/poster_db",
      DB_MAX_OPEN_CONNS: "12",
      DB_CONN_MAX_IDLE_TIME: "30s",
      DB_CONN_MAX_LIFETIME: "0s",
    });

    const poolConfig = postgresPoolConfigFromConfig(config);

    expect(poolConfig).toMatchObject({
      connectionString: "postgres://postgres:postgres@db:5432/poster_db",
      max: 12,
      idleTimeoutMillis: 30_000,
      maxLifetimeSeconds: 0,
    });
  });

  it("rounds positive sub-millisecond pool durations up to one millisecond", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      DB_CONN_MAX_IDLE_TIME: "1ns",
      DB_CONN_MAX_LIFETIME: "400us",
    });

    const poolConfig = postgresPoolConfigFromConfig(config);

    expect(poolConfig).toMatchObject({
      idleTimeoutMillis: 1,
      maxLifetimeSeconds: 0.001,
    });
  });

  it("rejects invalid postgres pool durations", () => {
    expect(() =>
      loadConfig({
        NODE_ENV: "test",
        DB_CONN_MAX_IDLE_TIME: "300",
      }),
    ).toThrow();
  });

  it("enables encrypted postgres connections for require mode", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      DB_SSLMODE: "require",
    });

    const poolConfig = postgresPoolConfigFromConfig(config);

    expect(poolConfig.ssl).toEqual({ rejectUnauthorized: false });
  });

  it("enables certificate verification for verify-full mode", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      DB_SSLMODE: "verify-full",
    });

    const poolConfig = postgresPoolConfigFromConfig(config);

    expect(poolConfig.ssl).toEqual({ rejectUnauthorized: true });
  });
});

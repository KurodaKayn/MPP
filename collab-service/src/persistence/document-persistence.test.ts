import { Document } from "@hocuspocus/server";
import { describe, expect, it } from "vitest";
import { applyUpdate, encodeStateAsUpdate, encodeStateVector } from "yjs";

import { PostgresDocumentPersistence } from "./document-persistence.js";

interface QueryCall {
  text: string;
  values?: unknown[];
}

class FakeDatabase {
  calls: QueryCall[] = [];
  results: Record<string, unknown>[][] = [];

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

  it("flushes pending Yjs updates as a sequenced batch", async () => {
    const database = new FakeDatabase();
    database.results = [
      [
        {
          current_seq: 7,
        },
      ],
    ];
    const persistence = new PostgresDocumentPersistence(database, 10_000);
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
      expect.stringContaining("INSERT INTO collab_document_update_batches"),
      expect.stringContaining("UPDATE collab_documents"),
      "COMMIT",
    ]);
    const insertCall = database.calls[2];
    expect(insertCall?.values?.[0]).toBe(
      "11111111-1111-4111-8111-111111111111",
    );
    expect(insertCall?.values?.[1]).toBe(8);
    expect(insertCall?.values?.[2]).toBe(9);
    expect(insertCall?.values?.[4]).toBe(2);
    expect(insertCall?.values?.[6]).toBe(
      "33333333-3333-4333-8333-333333333333",
    );

    const restored = new Document("restored");
    applyUpdate(restored, new Uint8Array(insertCall?.values?.[3] as Buffer));
    expect(restored.getMap("content").get("title")).toBe("First");
    expect(restored.getMap("content").get("body")).toBe("Second");
  });

  it("upserts the current Yjs snapshot", async () => {
    const database = new FakeDatabase();
    database.results = [
      [
        {
          current_seq: 12,
        },
      ],
    ];
    const persistence = new PostgresDocumentPersistence(database);
    const document = new Document("document");
    document.getMap("content").set("title", "Saved");

    await persistence.store("11111111-1111-4111-8111-111111111111", document);

    expect(database.calls[0]?.text).toBe("BEGIN");
    const call = database.calls[2];
    expect(call?.text).toContain("y_doc_state");
    expect(call?.text).not.toContain("ydoc_state");
    expect(call?.text).toContain("ON CONFLICT (document_id) DO UPDATE");
    expect(call?.values?.[0]).toBe("11111111-1111-4111-8111-111111111111");
    expect(call?.values?.[1]).toBeInstanceOf(Buffer);
    expect(call?.values?.[2]).toBeInstanceOf(Buffer);
    const state = call?.values?.[1] as Buffer;
    expect(call?.values?.[3]).toBe(12);
    expect(call?.values?.[4]).toBe(state.length);
    expect(call?.values?.[2]).toEqual(Buffer.from(encodeStateVector(document)));
    expect(database.calls[3]?.text).toBe("COMMIT");
  });
});

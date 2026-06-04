import { Document } from "@hocuspocus/server";
import { describe, expect, it } from "vitest";
import { encodeStateAsUpdate } from "yjs";

import { PostgresDocumentPersistence } from "./document-persistence.js";

interface QueryCall {
  text: string;
  values?: unknown[];
}

class FakeDatabase {
  calls: QueryCall[] = [];
  rows: Record<string, unknown>[] = [];

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
    return {
      command: "SELECT",
      rowCount: this.rows.length,
      oid: 0,
      fields: [],
      rows: this.rows as Row[],
    };
  }
}

describe("PostgresDocumentPersistence", () => {
  it("loads a stored Yjs snapshot", async () => {
    const source = new Document("source");
    source.getMap("content").set("title", "Persisted");
    const database = new FakeDatabase();
    database.rows = [
      {
        ydoc_state: Buffer.from(encodeStateAsUpdate(source)),
      },
    ];
    const persistence = new PostgresDocumentPersistence(database);
    const target = new Document("target");

    await persistence.load("11111111-1111-4111-8111-111111111111", target);

    expect(target.getMap("content").get("title")).toBe("Persisted");
    expect(database.calls[0]?.values).toEqual([
      "11111111-1111-4111-8111-111111111111",
    ]);
  });

  it("upserts the current Yjs snapshot", async () => {
    const database = new FakeDatabase();
    const persistence = new PostgresDocumentPersistence(database);
    const document = new Document("document");
    document.getMap("content").set("title", "Saved");

    await persistence.store("11111111-1111-4111-8111-111111111111", document);

    const call = database.calls[0];
    expect(call?.text).toContain("ON CONFLICT (document_id) DO UPDATE");
    expect(call?.values?.[0]).toBe("11111111-1111-4111-8111-111111111111");
    expect(call?.values?.[1]).toBeInstanceOf(Buffer);
    expect(call?.values?.[2]).toBeInstanceOf(Buffer);
    const state = call?.values?.[1] as Buffer;
    expect(call?.values?.[3]).toBe(state.length);
  });
});

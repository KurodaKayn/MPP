import { Pool } from "pg";
import { applyUpdate, encodeStateAsUpdate, encodeStateVector } from "yjs";

import type { Document } from "@hocuspocus/server";
import type { PoolConfig, QueryResult } from "pg";
import type { CollabConfig } from "../config.js";

interface Queryable {
  query<Row extends Record<string, unknown>>(
    text: string,
    values?: unknown[],
  ): Promise<QueryResult<Row>>;
}

interface DocumentStateRow extends Record<string, unknown> {
  ydoc_state: Buffer;
}

export interface DocumentPersistence {
  load(documentId: string, document: Document): Promise<void>;
  store(documentId: string, document: Document): Promise<void>;
  ping(): Promise<void>;
  close(): Promise<void>;
}

export class PostgresDocumentPersistence implements DocumentPersistence {
  constructor(
    private readonly database: Queryable & { end?: () => Promise<void> },
  ) {}

  async load(documentId: string, document: Document): Promise<void> {
    const result = await this.database.query<DocumentStateRow>(
      `
        SELECT ydoc_state
        FROM collab_document_states
        WHERE document_id = $1
      `,
      [documentId],
    );

    const state = result.rows[0]?.ydoc_state;
    if (state && state.length > 0) {
      applyUpdate(document, new Uint8Array(state));
    }
  }

  async store(documentId: string, document: Document): Promise<void> {
    const state = Buffer.from(encodeStateAsUpdate(document));
    const stateVector = Buffer.from(encodeStateVector(document));

    await this.database.query(
      `
        INSERT INTO collab_document_states (
          document_id,
          ydoc_state,
          state_vector,
          state_size_bytes,
          updated_at
        )
        VALUES ($1, $2, $3, $4, NOW())
        ON CONFLICT (document_id) DO UPDATE SET
          ydoc_state = EXCLUDED.ydoc_state,
          state_vector = EXCLUDED.state_vector,
          state_size_bytes = EXCLUDED.state_size_bytes,
          updated_at = EXCLUDED.updated_at
      `,
      [documentId, state, stateVector, state.length],
    );
  }

  async ping(): Promise<void> {
    await this.database.query("SELECT 1");
  }

  async close(): Promise<void> {
    await this.database.end?.();
  }
}

export function createPostgresDocumentPersistence(
  config: CollabConfig,
): DocumentPersistence {
  const poolConfig: PoolConfig = config.DATABASE_URL
    ? { connectionString: config.DATABASE_URL }
    : {
        host: config.DB_HOST,
        port: config.DB_PORT,
        user: config.DB_USER,
        password: config.DB_PASSWORD,
        database: config.DB_NAME,
      };

  return new PostgresDocumentPersistence(new Pool(poolConfig));
}

import { Pool } from "pg";
import {
  applyUpdate,
  encodeStateAsUpdate,
  encodeStateVector,
  mergeUpdates,
} from "yjs";

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
  y_doc_state: Buffer;
  compacted_until_seq: string | number;
}

interface DocumentBatchRow extends Record<string, unknown> {
  update_payload: Buffer;
}

interface DocumentSeqRow extends Record<string, unknown> {
  current_seq: string | number;
}

export interface DocumentPersistence {
  load(documentId: string, document: Document): Promise<void>;
  appendUpdate(
    documentId: string,
    update: Uint8Array,
    actorUserId?: string,
  ): Promise<void>;
  store(documentId: string, document: Document): Promise<void>;
  flush(): Promise<void>;
  ping(): Promise<void>;
  close(): Promise<void>;
}

interface PendingUpdate {
  update: Uint8Array;
  actorUserId?: string;
}

interface PersistenceLogger {
  error(message?: unknown, ...optionalParams: unknown[]): void;
}

export class PostgresDocumentPersistence implements DocumentPersistence {
  private readonly pendingUpdates = new Map<string, PendingUpdate[]>();
  private readonly flushTimers = new Map<string, NodeJS.Timeout>();
  private readonly flushes = new Map<string, Promise<void>>();
  private readonly flushRetryAttempts = new Map<string, number>();

  constructor(
    private readonly database: Queryable & { end?: () => Promise<void> },
    private readonly flushIntervalMs = 300,
    private readonly maxUpdatesPerBatch = 32,
    private readonly updateRetentionDays = 30,
    private readonly maxFlushRetryAttempts = 5,
    private readonly maxFlushRetryDelayMs = 30_000,
    private readonly logger: PersistenceLogger = console,
  ) {}

  async load(documentId: string, document: Document): Promise<void> {
    const result = await this.database.query<DocumentStateRow>(
      `
        SELECT y_doc_state, compacted_until_seq
        FROM collab_document_states
        WHERE document_id = $1
      `,
      [documentId],
    );

    const stateRow = result.rows[0];
    const state = stateRow?.y_doc_state;
    if (state && state.length > 0) {
      applyUpdate(document, new Uint8Array(state));
    }

    const compactedUntilSeq = Number(stateRow?.compacted_until_seq ?? 0);
    const batches = await this.database.query<DocumentBatchRow>(
      `
        SELECT update_payload
        FROM collab_document_update_batches
        WHERE document_id = $1
          AND to_seq > $2
        ORDER BY from_seq ASC
      `,
      [documentId, compactedUntilSeq],
    );
    for (const batch of batches.rows) {
      if (batch.update_payload.length > 0) {
        applyUpdate(document, new Uint8Array(batch.update_payload));
      }
    }
  }

  async appendUpdate(
    documentId: string,
    update: Uint8Array,
    actorUserId?: string,
  ): Promise<void> {
    const pending = this.pendingUpdates.get(documentId) ?? [];
    pending.push({ update: new Uint8Array(update), actorUserId });
    this.pendingUpdates.set(documentId, pending);
    this.flushRetryAttempts.delete(documentId);

    if (pending.length >= this.maxUpdatesPerBatch) {
      try {
        await this.flushDocument(documentId);
      } catch (error) {
        this.handleFlushFailure(documentId, error);
      }
      return;
    }

    this.scheduleFlush(documentId);
  }

  async store(documentId: string, document: Document): Promise<void> {
    await this.flushDocument(documentId);

    const state = Buffer.from(encodeStateAsUpdate(document));
    const stateVector = Buffer.from(encodeStateVector(document));

    await this.database.query("BEGIN");
    try {
      const seq = await this.lockDocumentAndReadSeq(documentId);
      await this.database.query(
        `
          INSERT INTO collab_document_states (
            document_id,
            y_doc_state,
            state_vector,
            compacted_until_seq,
            state_size_bytes,
            updated_at
          )
          VALUES ($1, $2, $3, $4, $5, NOW())
          ON CONFLICT (document_id) DO UPDATE SET
            y_doc_state = EXCLUDED.y_doc_state,
            state_vector = EXCLUDED.state_vector,
            compacted_until_seq = EXCLUDED.compacted_until_seq,
            state_size_bytes = EXCLUDED.state_size_bytes,
            updated_at = EXCLUDED.updated_at
        `,
        [documentId, state, stateVector, seq, state.length],
      );
      await this.database.query("COMMIT");
      await this.pruneCompactedBatches(documentId, seq).catch((error) => {
        this.logger.error("failed to prune compacted collab update batches", {
          documentId,
          error,
        });
      });
    } catch (error) {
      await this.database.query("ROLLBACK");
      throw error;
    }
  }

  async flush(): Promise<void> {
    await Promise.all(this.flushes.values());

    const pendingDocumentIds = Array.from(this.pendingUpdates.keys());
    await Promise.all(
      pendingDocumentIds.map((documentId) => this.flushDocument(documentId)),
    );
  }

  async ping(): Promise<void> {
    await this.database.query("SELECT 1");
  }

  async close(): Promise<void> {
    await this.flush();
    await this.database.end?.();
  }

  private scheduleFlush(
    documentId: string,
    delayMs = this.flushIntervalMs,
  ): void {
    if (this.flushTimers.has(documentId)) {
      return;
    }

    const timer = setTimeout(() => {
      this.flushTimers.delete(documentId);
      void this.flushDocument(documentId).catch((error) => {
        this.handleFlushFailure(documentId, error);
      });
    }, delayMs);
    this.flushTimers.set(documentId, timer);
  }

  private handleFlushFailure(documentId: string, error?: unknown): void {
    if ((this.pendingUpdates.get(documentId)?.length ?? 0) === 0) {
      this.flushRetryAttempts.delete(documentId);
      return;
    }

    const attempt = (this.flushRetryAttempts.get(documentId) ?? 0) + 1;
    this.flushRetryAttempts.set(documentId, attempt);
    this.logger.error("failed to flush collab update batch", {
      documentId,
      attempt,
      maxAttempts: this.maxFlushRetryAttempts,
      error,
    });

    if (attempt >= this.maxFlushRetryAttempts) {
      this.logger.error("collab update batch retries exhausted", {
        documentId,
        attempts: attempt,
      });
      return;
    }

    const retryDelayMs = Math.min(
      this.flushIntervalMs * 2 ** (attempt - 1),
      this.maxFlushRetryDelayMs,
    );
    this.scheduleFlush(documentId, retryDelayMs);
  }

  private async flushDocument(documentId: string): Promise<void> {
    const existingFlush = this.flushes.get(documentId);
    if (existingFlush) {
      await existingFlush;
    }

    const flush = this.flushDocumentOnce(documentId).finally(() => {
      if (this.flushes.get(documentId) === flush) {
        this.flushes.delete(documentId);
      }
    });
    this.flushes.set(documentId, flush);
    await flush;
  }

  private async flushDocumentOnce(documentId: string): Promise<void> {
    const timer = this.flushTimers.get(documentId);
    if (timer) {
      clearTimeout(timer);
      this.flushTimers.delete(documentId);
    }

    const pending = this.pendingUpdates.get(documentId);
    if (!pending || pending.length === 0) {
      return;
    }
    this.pendingUpdates.delete(documentId);

    const payload = Buffer.from(
      mergeUpdates(pending.map(({ update }) => update)),
    );
    const actorUserId = lastActorUserId(pending);

    await this.database.query("BEGIN");
    try {
      const currentSeq = await this.lockDocumentAndReadSeq(documentId);
      const fromSeq = currentSeq + 1;
      const toSeq = currentSeq + pending.length;
      await this.database.query(
        `
          INSERT INTO collab_document_update_batches (
            document_id,
            from_seq,
            to_seq,
            update_payload,
            update_count,
            payload_size_bytes,
            actor_user_id,
            created_at
          )
          VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
        `,
        [
          documentId,
          fromSeq,
          toSeq,
          payload,
          pending.length,
          payload.length,
          actorUserId ?? null,
        ],
      );
      await this.database.query(
        `
          UPDATE collab_documents
          SET current_seq = $2,
              last_edited_by = COALESCE($3, last_edited_by),
              last_edited_at = NOW(),
              updated_at = NOW()
          WHERE id = $1
        `,
        [documentId, toSeq, actorUserId ?? null],
      );
      await this.database.query("COMMIT");
      this.flushRetryAttempts.delete(documentId);
    } catch (error) {
      await this.database.query("ROLLBACK");
      this.pendingUpdates.set(documentId, [
        ...pending,
        ...(this.pendingUpdates.get(documentId) ?? []),
      ]);
      throw error;
    }
  }

  private async lockDocumentAndReadSeq(documentId: string): Promise<number> {
    const result = await this.database.query<DocumentSeqRow>(
      `
        SELECT current_seq
        FROM collab_documents
        WHERE id = $1
        FOR UPDATE
      `,
      [documentId],
    );

    const row = result.rows[0];
    if (!row) {
      throw new Error("collaborative document not found");
    }
    return Number(row.current_seq);
  }

  private async pruneCompactedBatches(
    documentId: string,
    compactedUntilSeq: number,
  ): Promise<void> {
    await this.database.query(
      `
        DELETE FROM collab_document_update_batches
        WHERE document_id = $1
          AND to_seq <= $2
          AND created_at < NOW() - make_interval(days => $3)
      `,
      [documentId, compactedUntilSeq, this.updateRetentionDays],
    );
  }
}

function lastActorUserId(pending: PendingUpdate[]): string | undefined {
  for (let index = pending.length - 1; index >= 0; index -= 1) {
    const actorUserId = pending[index]?.actorUserId;
    if (actorUserId) {
      return actorUserId;
    }
  }
  return undefined;
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

  return new PostgresDocumentPersistence(
    new Pool(poolConfig),
    config.COLLAB_UPDATE_FLUSH_MS,
    config.COLLAB_UPDATE_FLUSH_MAX_COUNT,
    config.COLLAB_UPDATE_RETENTION_DAYS,
    config.COLLAB_UPDATE_FLUSH_RETRY_MAX_ATTEMPTS,
    config.COLLAB_UPDATE_FLUSH_RETRY_MAX_MS,
  );
}

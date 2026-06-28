import { readFileSync } from "node:fs";
import { Pool } from "pg";
import { Document } from "@hocuspocus/server";
import {
  applyUpdate,
  encodeStateAsUpdate,
  encodeStateVector,
  mergeUpdates,
} from "yjs";

import {
  createProjectYDoc,
  projectYDocToHtml,
} from "../collab/project-document.js";

import type { PoolConfig, QueryResult } from "pg";
import type { ConnectionOptions } from "node:tls";
import type { CollabConfig } from "../config.js";
import type { Metrics } from "../metrics.js";

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

interface ProjectDocumentRow extends Record<string, unknown> {
  workspace_id: string;
  source_content: string;
  current_seq: string | number;
  has_state: boolean;
  has_updates: boolean;
}

interface DocumentWorkspaceRow extends Record<string, unknown> {
  workspace_id: string;
}

export interface DocumentPersistence {
  load(documentId: string, document: Document): Promise<void>;
  initializeProjectDocument(documentId: string): Promise<boolean>;
  syncProjectSourceContent(documentId: string): Promise<boolean>;
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
    private readonly metrics?: Pick<Metrics, "recordUpdateFlush">,
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

  async initializeProjectDocument(documentId: string): Promise<boolean> {
    const result = await this.database.query<ProjectDocumentRow>(
      `
        SELECT
          collab_documents.workspace_id,
          projects.source_content,
          collab_documents.current_seq,
          EXISTS (
            SELECT 1
            FROM collab_document_states
            WHERE document_id = $1
          ) AS has_state,
          EXISTS (
            SELECT 1
            FROM collab_document_update_batches
            WHERE document_id = $1
          ) AS has_updates
        FROM projects
        JOIN collab_documents
          ON collab_documents.id = projects.collab_document_id
        WHERE projects.collab_document_id = $1
      `,
      [documentId],
    );

    const row = result.rows[0];
    if (!row) {
      return false;
    }
    if (row.has_state || row.has_updates) {
      return true;
    }

    const currentSeq = Number(row.current_seq);
    if (currentSeq !== 0) {
      throw new Error("collaborative document state is incomplete");
    }

    const document = createProjectYDoc(row.source_content);
    try {
      const state = Buffer.from(encodeStateAsUpdate(document));
      const stateVector = Buffer.from(encodeStateVector(document));
      await this.database.query(
        `
          INSERT INTO collab_document_states (
            document_id,
            workspace_id,
            y_doc_state,
            state_vector,
            compacted_until_seq,
            state_size_bytes,
            updated_at
          )
          VALUES ($1, $2, $3, $4, $5, $6, NOW())
          ON CONFLICT (document_id) DO NOTHING
        `,
        [
          documentId,
          row.workspace_id,
          state,
          stateVector,
          currentSeq,
          state.length,
        ],
      );
    } finally {
      document.destroy();
    }

    return true;
  }

  async syncProjectSourceContent(documentId: string): Promise<boolean> {
    const initialized = await this.initializeProjectDocument(documentId);
    if (!initialized) {
      return false;
    }

    await this.flushDocument(documentId);

    const document = new Document(documentId);
    try {
      await this.load(documentId, document);
      const sourceContent = projectYDocToHtml(document);

      await this.database.query("BEGIN");
      try {
        await this.database.query(
          `
            UPDATE projects
            SET source_content = $2,
                updated_at = NOW()
            WHERE collab_document_id = $1
              AND source_content IS DISTINCT FROM $2
          `,
          [documentId, sourceContent],
        );
        await this.database.query("COMMIT");
      } catch (error) {
        await this.database.query("ROLLBACK");
        throw error;
      }
    } finally {
      document.destroy();
    }

    return true;
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
      const workspaceId = await this.documentWorkspaceId(documentId);
      await this.database.query(
        `
          INSERT INTO collab_document_states (
            document_id,
            workspace_id,
            y_doc_state,
            state_vector,
            compacted_until_seq,
            state_size_bytes,
            updated_at
          )
          VALUES ($1, $2, $3, $4, $5, $6, NOW())
          ON CONFLICT (document_id) DO UPDATE SET
            workspace_id = EXCLUDED.workspace_id,
            y_doc_state = EXCLUDED.y_doc_state,
            state_vector = EXCLUDED.state_vector,
            compacted_until_seq = EXCLUDED.compacted_until_seq,
            state_size_bytes = EXCLUDED.state_size_bytes,
            updated_at = EXCLUDED.updated_at
        `,
        [documentId, workspaceId, state, stateVector, seq, state.length],
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

    if (attempt === this.maxFlushRetryAttempts) {
      this.logger.error(
        "collab update batch retry threshold reached; continuing capped retries",
        {
          documentId,
          attempts: attempt,
        },
      );
    } else if (attempt > this.maxFlushRetryAttempts) {
      this.logger.error("collab update batch retry still failing", {
        documentId,
        attempts: attempt,
      });
    }

    const backoffAttempt = Math.min(
      attempt - 1,
      this.maxFlushRetryAttempts - 1,
    );
    const retryDelayMs = Math.min(
      this.flushIntervalMs * 2 ** backoffAttempt,
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
    const startedAt = process.hrtime.bigint();
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
      const workspaceId = await this.documentWorkspaceId(documentId);
      const fromSeq = currentSeq + 1;
      const toSeq = currentSeq + pending.length;
      await this.database.query(
        `
          INSERT INTO collab_document_update_batches (
            document_id,
            workspace_id,
            from_seq,
            to_seq,
            update_payload,
            update_count,
            payload_size_bytes,
            actor_user_id,
            created_at
          )
          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
        `,
        [
          documentId,
          workspaceId,
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
    } finally {
      const elapsedNs = process.hrtime.bigint() - startedAt;
      this.metrics?.recordUpdateFlush(Number(elapsedNs) / 1_000_000_000);
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

  private async documentWorkspaceId(documentId: string): Promise<string> {
    const result = await this.database.query<DocumentWorkspaceRow>(
      `
        SELECT workspace_id
        FROM collab_documents
        WHERE id = $1
      `,
      [documentId],
    );

    const workspaceId = result.rows[0]?.workspace_id;
    if (!workspaceId) {
      throw new Error("collaborative document workspace not found");
    }
    return workspaceId;
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
  metrics?: Pick<Metrics, "recordUpdateFlush">,
): DocumentPersistence {
  return new PostgresDocumentPersistence(
    new Pool(postgresPoolConfigFromConfig(config)),
    config.COLLAB_UPDATE_FLUSH_MS,
    config.COLLAB_UPDATE_FLUSH_MAX_COUNT,
    config.COLLAB_UPDATE_RETENTION_DAYS,
    config.COLLAB_UPDATE_FLUSH_RETRY_MAX_ATTEMPTS,
    config.COLLAB_UPDATE_FLUSH_RETRY_MAX_MS,
    console,
    metrics,
  );
}

export function postgresPoolConfigFromConfig(config: CollabConfig): PoolConfig {
  const poolConfig: PoolConfig = config.DATABASE_URL
    ? { connectionString: config.DATABASE_URL }
    : {
        host: config.DB_HOST,
        port: config.DB_PORT,
        user: config.DB_USER,
        password: config.DB_PASSWORD,
        database: config.DB_NAME,
      };
  poolConfig.max = config.DB_MAX_OPEN_CONNS;
  poolConfig.idleTimeoutMillis = config.DB_CONN_MAX_IDLE_TIME;
  poolConfig.maxLifetimeSeconds = durationMillisToSeconds(
    config.DB_CONN_MAX_LIFETIME,
  );
  const ssl = postgresSSLConfigFromConfig(config);
  if (ssl !== undefined) {
    poolConfig.ssl = ssl;
  }
  return poolConfig;
}

function durationMillisToSeconds(millis: number): number {
  if (millis <= 0) {
    return 0;
  }
  return millis / 1_000;
}

function postgresSSLConfigFromConfig(
  config: CollabConfig,
): PoolConfig["ssl"] | undefined {
  switch (config.DB_SSLMODE) {
    case "disable":
    case "allow":
    case "prefer":
      return undefined;
    case "require":
      return { rejectUnauthorized: false };
    case "verify-ca":
      return {
        ...postgresSSLCAConfig(config),
        rejectUnauthorized: true,
        checkServerIdentity: () => undefined,
      } satisfies ConnectionOptions;
    case "verify-full":
      return {
        ...postgresSSLCAConfig(config),
        rejectUnauthorized: true,
      } satisfies ConnectionOptions;
  }
}

function postgresSSLCAConfig(
  config: CollabConfig,
): Pick<ConnectionOptions, "ca"> {
  const sslRootCert = config.DB_SSLROOTCERT?.trim();
  if (!sslRootCert) {
    return {};
  }
  return { ca: readFileSync(sslRootCert, "utf8") };
}

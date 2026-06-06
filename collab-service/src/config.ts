import { z } from "zod";

const EnvBoolean = z
  .union([z.boolean(), z.string()])
  .transform((value) =>
    typeof value === "boolean"
      ? value
      : ["1", "true", "yes", "on"].includes(value.trim().toLowerCase()),
  );

const EnvSchema = z.object({
  NODE_ENV: z
    .enum(["development", "test", "production"])
    .default("development"),
  LOG_LEVEL: z.string().default("info"),
  COLLAB_HOST: z.string().default("0.0.0.0"),
  COLLAB_PORT: z.coerce.number().int().positive().default(8090),
  COLLAB_WS_PATH: z.string().default("/collab/documents/:documentId"),
  COLLAB_HEARTBEAT_SECONDS: z.coerce.number().int().positive().default(30),
  COLLAB_UPDATE_FLUSH_MS: z.coerce.number().int().positive().default(300),
  COLLAB_UPDATE_FLUSH_MAX_MS: z.coerce.number().int().positive().default(2_000),
  COLLAB_UPDATE_FLUSH_MAX_COUNT: z.coerce.number().int().positive().default(32),
  COLLAB_UPDATE_FLUSH_RETRY_MAX_ATTEMPTS: z.coerce
    .number()
    .int()
    .positive()
    .default(5),
  COLLAB_UPDATE_FLUSH_RETRY_MAX_MS: z.coerce
    .number()
    .int()
    .positive()
    .default(30_000),
  COLLAB_UPDATE_RETENTION_DAYS: z.coerce.number().int().positive().default(30),
  DATABASE_URL: z.string().min(1).optional(),
  DB_HOST: z.string().default("db"),
  DB_PORT: z.coerce.number().int().positive().default(5432),
  DB_USER: z.string().default("postgres"),
  DB_PASSWORD: z.string().default("postgres"),
  DB_NAME: z.string().default("poster_db"),
  DB_SSLMODE: z
    .enum(["disable", "allow", "prefer", "require", "verify-ca", "verify-full"])
    .default("disable"),
  DB_SSLROOTCERT: z.string().optional(),
  REDIS_ADDR: z.string().default("redis:6379"),
  REDIS_PASSWORD: z.string().default(""),
  REDIS_DB: z.coerce.number().int().nonnegative().default(0),
  REDIS_TLS: EnvBoolean.default(false),
  COLLAB_REDIS_SYNC_ENABLED: EnvBoolean.default(true),
  COLLAB_REDIS_CHANNEL_PREFIX: z.string().default("mpp:collab:doc"),
  BACKEND_INTERNAL_URL: z.string().url().default("http://backend:8080"),
  COLLAB_TOKEN_SECRET: z.string().optional(),
});

export type CollabConfig = z.infer<typeof EnvSchema>;

export function loadConfig(env: NodeJS.ProcessEnv = process.env): CollabConfig {
  return EnvSchema.parse(env);
}

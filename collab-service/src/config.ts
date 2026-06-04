import { z } from "zod";

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
  DATABASE_URL: z.string().optional(),
  REDIS_ADDR: z.string().default("redis:6379"),
  BACKEND_INTERNAL_URL: z.string().url().default("http://backend:8080"),
  COLLAB_TOKEN_SECRET: z.string().optional(),
});

export type CollabConfig = z.infer<typeof EnvSchema>;

export function loadConfig(env: NodeJS.ProcessEnv = process.env): CollabConfig {
  return EnvSchema.parse(env);
}

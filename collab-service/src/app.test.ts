import { describe, expect, it } from "vitest";

import { buildApp } from "./app.js";
import { loadConfig } from "./config.js";

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
    const app = await buildApp(testConfig());

    const health = await app.inject("/health");
    const ready = await app.inject("/ready");

    expect(health.statusCode).toBe(200);
    expect(health.json()).toEqual({ status: "healthy" });
    expect(ready.statusCode).toBe(200);
    expect(ready.json()).toMatchObject({
      status: "ready",
      dependencies: {
        database_configured: false,
        redis_addr: "redis:6379",
        token_secret_configured: true,
      },
    });

    await app.close();
  });

  it("serves prometheus metrics", async () => {
    const app = await buildApp(testConfig());

    const response = await app.inject("/metrics");

    expect(response.statusCode).toBe(200);
    expect(response.headers["content-type"]).toContain("text/plain");
    expect(response.body).toContain("collab_service_info");

    await app.close();
  });
});

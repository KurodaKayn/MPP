#!/usr/bin/env node
import { setTimeout as sleep } from "node:timers/promises";

const primary = new URL(
  process.env.COLLAB_PRIMARY_URL ?? "http://127.0.0.1:8090",
);
const secondary = new URL(
  process.env.COLLAB_SECONDARY_URL ?? "http://127.0.0.1:8091",
);
const documentId =
  process.env.COLLAB_DOCUMENT_ID ?? "11111111-1111-4111-8111-111111111111";
const token = process.env.COLLAB_TOKEN;
const connections = Number.parseInt(process.env.COLLAB_LOAD_CONNECTIONS ?? "16", 10);
const holdMs = Number.parseInt(process.env.COLLAB_LOAD_HOLD_MS ?? "5000", 10);

if (!globalThis.WebSocket) {
  throw new Error("Node.js 22+ is required because this script uses global WebSocket");
}

await assertReady(primary);
await assertReady(secondary);

const sockets = await Promise.all(
  Array.from({ length: connections }, (_, index) =>
    openSocket(index % 2 === 0 ? primary : secondary, token),
  ),
);

await sleep(holdMs);
await assertMetric(primary, "collab_active_connections");
await assertMetric(primary, "collab_active_documents");
await assertMetric(primary, "collab_update_flush_latency_seconds");
await assertAuthDenial(primary);

for (const socket of sockets) {
  socket.close();
}

console.log(
  `ok - opened ${sockets.length} websocket connections across two collab-service URLs`,
);

async function assertReady(baseUrl) {
  const response = await fetch(new URL("/ready", baseUrl));
  if (!response.ok) {
    throw new Error(`${baseUrl} readiness returned ${response.status}`);
  }
  const body = await response.json();
  if (body.redis_sync !== "ready") {
    throw new Error(`${baseUrl} redis_sync is ${body.redis_sync}`);
  }
}

async function assertMetric(baseUrl, metricName) {
  const metrics = await fetchText(new URL("/metrics", baseUrl));
  if (!metrics.includes(metricName)) {
    throw new Error(`${baseUrl} metrics missing ${metricName}`);
  }
}

async function assertAuthDenial(baseUrl) {
  const before = await metricText(baseUrl, "collab_auth_denials_total");
  await expectSocketFailure(baseUrl);
  await sleep(250);
  const after = await metricText(baseUrl, "collab_auth_denials_total");
  if (before === after) {
    throw new Error(`${baseUrl} auth denial metric did not change`);
  }
}

async function metricText(baseUrl, metricName) {
  return (await fetchText(new URL("/metrics", baseUrl)))
    .split("\n")
    .filter((line) => line.startsWith(metricName))
    .join("\n");
}

async function fetchText(url) {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`${url} returned ${response.status}`);
  }
  return response.text();
}

function openSocket(baseUrl, authToken) {
  if (!authToken) {
    throw new Error("COLLAB_TOKEN is required for authenticated load sockets");
  }
  return connect(collabWsUrl(baseUrl, authToken), true);
}

async function expectSocketFailure(baseUrl) {
  const socket = await connect(collabWsUrl(baseUrl, "invalid-token"), false);
  socket.close();
}

function collabWsUrl(baseUrl, authToken) {
  const url = new URL(`/collab/documents/${documentId}`, baseUrl);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.searchParams.set("token", authToken);
  return url;
}

function connect(url, expectOpen) {
  return new Promise((resolve, reject) => {
    const socket = new WebSocket(url);
    const timer = setTimeout(() => {
      socket.close();
      reject(new Error(`${url} websocket timed out`));
    }, 5000);

    socket.addEventListener("open", () => {
      clearTimeout(timer);
      if (expectOpen) {
        resolve(socket);
        return;
      }
      reject(new Error(`${url} unexpectedly opened`));
    });
    socket.addEventListener("close", () => {
      clearTimeout(timer);
      if (!expectOpen) {
        resolve(socket);
      }
    });
    socket.addEventListener("error", (event) => {
      clearTimeout(timer);
      if (!expectOpen) {
        resolve(socket);
        return;
      }
      reject(event.error ?? new Error(`${url} websocket failed`));
    });
  });
}

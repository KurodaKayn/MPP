#!/usr/bin/env node
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const composePath = resolve(process.cwd(), "docker/docker-compose.yml");
const compose = readFileSync(composePath, "utf8");

const checks = [
  [
    "collab service exposes the /collab websocket route",
    /traefik\.http\.routers\.collab-web\.rule=PathPrefix\(`\/collab`\)/,
  ],
  [
    "collab secure route exposes the /collab websocket route",
    /traefik\.http\.routers\.collab-websecure\.rule=PathPrefix\(`\/collab`\)/,
  ],
  [
    "collab route has priority above the frontend catch-all",
    /traefik\.http\.routers\.collab-web\.priority=100/,
  ],
  [
    "collab secure route has priority above the frontend catch-all",
    /traefik\.http\.routers\.collab-websecure\.priority=100/,
  ],
  [
    "collab traefik service points at the websocket server port",
    /traefik\.http\.services\.collab\.loadbalancer\.server\.port=8090/,
  ],
  [
    "traefik waits for collab-service health before routing",
    /traefik:[\s\S]*?depends_on:[\s\S]*?collab-service:[\s\S]*?condition: service_healthy/,
  ],
  [
    "collab-service waits for redis health before starting",
    /collab-service:[\s\S]*?depends_on:[\s\S]*?redis:[\s\S]*?condition: service_healthy/,
  ],
];

let failed = false;
for (const [name, pattern] of checks) {
  if (pattern.test(compose)) {
    console.log(`ok - ${name}`);
    continue;
  }
  failed = true;
  console.error(`not ok - ${name}`);
}

if (failed) {
  process.exit(1);
}

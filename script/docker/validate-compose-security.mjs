#!/usr/bin/env node
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const root = process.cwd();
const compose = readFileSync(resolve(root, "docker/docker-compose.yml"), "utf8");
const devCompose = readFileSync(
  resolve(root, "docker/docker-compose.dev.yml"),
  "utf8",
);

const checks = [
  [
    "backend receives browser-worker internal token",
    /backend:[\s\S]*?environment:[\s\S]*?BROWSER_WORKER_INTERNAL_TOKEN:\s*\$\{BROWSER_WORKER_INTERNAL_TOKEN:-\}/,
    compose,
  ],
  [
    "publish-worker receives browser-worker internal token",
    /publish-worker:[\s\S]*?environment:[\s\S]*?BROWSER_WORKER_INTERNAL_TOKEN:\s*\$\{BROWSER_WORKER_INTERNAL_TOKEN:-\}/,
    compose,
  ],
  [
    "browser-worker requires an internal token env",
    /browser-worker:[\s\S]*?environment:[\s\S]*?BROWSER_WORKER_INTERNAL_TOKEN:\s*\$\{BROWSER_WORKER_INTERNAL_TOKEN:-\}/,
    compose,
  ],
  [
    "browser-worker dev port is loopback-only",
    /browser-worker:[\s\S]*?ports:[\s\S]*?-\s+"127\.0\.0\.1:8081:8081"/,
    devCompose,
  ],
  [
    "backend receives ai-service internal token",
    /backend:[\s\S]*?environment:[\s\S]*?AI_SERVICE_INTERNAL_TOKEN:\s*\$\{AI_SERVICE_INTERNAL_TOKEN:-\}/,
    compose,
  ],
  [
    "publish-worker receives ai-service internal token",
    /publish-worker:[\s\S]*?environment:[\s\S]*?AI_SERVICE_INTERNAL_TOKEN:\s*\$\{AI_SERVICE_INTERNAL_TOKEN:-\}/,
    compose,
  ],
  [
    "ai-service receives internal token env",
    /ai-service:[\s\S]*?environment:[\s\S]*?AI_SERVICE_INTERNAL_TOKEN:\s*\$\{AI_SERVICE_INTERNAL_TOKEN:-\}/,
    compose,
  ],
  [
    "ai-service dev port is loopback-only",
    /ai-service:[\s\S]*?ports:[\s\S]*?-\s+"127\.0\.0\.1:8000:8000"/,
    devCompose,
  ],
];

let failed = false;
for (const [name, pattern, source] of checks) {
  if (pattern.test(source)) {
    console.log(`ok - ${name}`);
    continue;
  }
  failed = true;
  console.error(`not ok - ${name}`);
}

if (failed) {
  process.exit(1);
}

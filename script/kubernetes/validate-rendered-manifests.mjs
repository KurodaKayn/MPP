#!/usr/bin/env node

import { readFileSync } from "node:fs";

const [packageDir, renderedPath] = process.argv.slice(2);

if (!packageDir || !renderedPath) {
  console.error(
    "Usage: validate-rendered-manifests.mjs <package-dir> <rendered-yaml>",
  );
  process.exit(2);
}

const normalizedPackageDir = packageDir.replace(/\\/g, "/");
const rendered = readFileSync(renderedPath, "utf8");
const errors = [];

function addError(message) {
  errors.push(message);
}

function requireMatch(pattern, message) {
  if (!pattern.test(rendered)) {
    addError(message);
  }
}

const deployablePackage = isDeployablePackage(normalizedPackageDir);

const localImages = renderedImageIssues((image) => image.startsWith("mpp-"));
if (localImages.length > 0) {
  addError(`rendered manifests contain local app images: ${localImages.join("; ")}`);
}

const latestImages = renderedImageIssues((image) => image.endsWith(":latest"));
if (latestImages.length > 0) {
  addError(`rendered manifests contain latest image tags: ${latestImages.join("; ")}`);
}

const placeholderImages = deployablePackage
  ? renderedImageIssues(isPlaceholderImageValue)
  : [];
if (placeholderImages.length > 0) {
  addError(`rendered manifests contain placeholder app images: ${placeholderImages.join("; ")}`);
}

for (const issue of runtimeImageIssues({ rejectPlaceholders: deployablePackage })) {
  addError(issue);
}

if (isPathSuffix(normalizedPackageDir, "validation/app-baseline")) {
  validateAppBaselineOverlay();
}

if (errors.length > 0) {
  console.error(errors.join("\n"));
  process.exit(1);
}

function findLines(pattern) {
  return rendered
    .split("\n")
    .flatMap((line, index) => (pattern.test(line) ? [`${index + 1}:${line}`] : []));
}

function renderedImageIssues(isIssue) {
  return imageLines()
    .filter(({ value }) => isIssue(value))
    .map(({ lineNumber, line }) => `${lineNumber}:${line}`);
}

function imageLines() {
  return rendered.split("\n").flatMap((line, index) => {
    const match = line.match(/^\s*image:\s*([^#\s]+)\s*(?:#.*)?$/);
    if (!match) {
      return [];
    }

    return [
      {
        lineNumber: index + 1,
        line,
        value: unquoteYamlScalar(match[1]),
      },
    ];
  });
}

function unquoteYamlScalar(value) {
  const first = value.at(0);
  const last = value.at(-1);
  if ((first === `"` && last === `"`) || (first === `'` && last === `'`)) {
    return value.slice(1, -1);
  }
  return value;
}

function runtimeImageIssues({ rejectPlaceholders }) {
  const issues = [];
  const lines = rendered.split("\n");
  for (let index = 0; index < lines.length; index += 1) {
    if (!/^\s*-\s+name:\s+BROWSER_RUNTIME_IMAGE\s*$/.test(lines[index])) {
      continue;
    }

    for (let cursor = index + 1; cursor < lines.length; cursor += 1) {
      if (/^\s*-\s+name:/.test(lines[cursor])) {
        break;
      }

      const match = lines[cursor].match(/^\s*value:\s*([^#\s]+)\s*(?:#.*)?$/);
      if (!match) {
        continue;
      }

      const value = unquoteYamlScalar(match[1]);
      if (value.startsWith("mpp-") || value.endsWith(":latest")) {
        issues.push(
          `BROWSER_RUNTIME_IMAGE is unresolved at line ${cursor + 1}: ${lines[cursor]}`,
        );
      }
      if (rejectPlaceholders && isPlaceholderImageValue(value)) {
        issues.push(
          `BROWSER_RUNTIME_IMAGE has a placeholder value at line ${cursor + 1}: ${lines[cursor]}`,
        );
      }
      break;
    }
  }
  return issues;
}

function isPlaceholderImageValue(value) {
  return value.includes("replace-me") || value.startsWith("registry.example.invalid/");
}

function isDeployablePackage(dir) {
  return (
    !isPathSuffix(dir, "deploy/kubernetes/app-baseline") &&
    !isPathSuffix(dir, "deploy/kubernetes/browser-runtime-control") &&
    !dir.startsWith("validation/") &&
    !dir.includes("/validation/")
  );
}

function isPathSuffix(dir, suffix) {
  return dir === suffix || dir.endsWith(`/${suffix}`);
}

function validateAppBaselineOverlay() {
  const placeholderValues = findLines(/replace-me|replace-with-/);
  if (placeholderValues.length > 0) {
    addError(
      `validation overlay still contains placeholders: ${placeholderValues.join("; ")}`,
    );
  }

  requireMatch(/^\s*kind:\s*Secret\s*$/m, "validation overlay is missing a Secret");
  requireMatch(
    /^\s*name:\s*mpp-app-secrets\s*$/m,
    "validation overlay is missing mpp-app-secrets",
  );

  for (const key of [
    "JWT_SECRET",
    "DB_PASSWORD",
    "COLLAB_TOKEN_SECRET",
    "COOKIE_ENCRYPTION_KEY",
    "LLM_PROVIDER_KEY",
  ]) {
    requireMatch(new RegExp(`^\\s*${key}:`, "m"), `validation overlay mpp-app-secrets is missing ${key}`);
  }

  for (const service of ["postgres", "redis"]) {
    requireMatch(
      new RegExp(`^\\s*name:\\s*${service}\\s*$`, "m"),
      `validation overlay is missing ${service} Service`,
    );
  }

  const requiredConfig = new Map([
    ["DB_HOST", "postgres"],
    ["DB_SSLMODE", "verify-full"],
    ["REDIS_ADDR", "redis:6379"],
    ["COLLAB_WEBSOCKET_URL_BASE", "wss://mpp.example.invalid"],
    ["LLM_PROVIDER_URL", "https://llm.example.invalid/v1"],
    ["LLM_MODEL", "validation-model"],
  ]);

  for (const [key, value] of requiredConfig) {
    requireMatch(
      new RegExp(`^\\s*${key}:\\s*${escapeRegExp(value)}\\s*$`, "m"),
      `validation overlay mpp-app-config ${key} is not overridden`,
    );
  }

  requireMatch(
    /^\s*-\s*host:\s*mpp\.example\.invalid\s*$/m,
    "validation overlay Ingress host is not overridden",
  );
  requireMatch(
    /^\s*-\s*mpp\.example\.invalid\s*$/m,
    "validation overlay Ingress TLS host is not overridden",
  );
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

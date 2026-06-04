import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, describe, expect, it } from "vitest";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const extensionRoot = path.resolve(__dirname, "..");
const scriptPath = path.join(__dirname, "lint-chinese.mjs");
const tempRoots = [];

afterEach(() => {
  for (const root of tempRoots.splice(0)) {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

function createFixture(files) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "lint-chinese-"));
  tempRoots.push(root);

  for (const [relativePath, content] of Object.entries(files)) {
    const fullPath = path.join(root, relativePath);
    fs.mkdirSync(path.dirname(fullPath), { recursive: true });
    fs.writeFileSync(fullPath, content, "utf8");
  }

  return root;
}

function runLint(root) {
  return spawnSync(process.execPath, [scriptPath], {
    cwd: extensionRoot,
    env: {
      ...process.env,
      LINT_CHINESE_ROOT: root,
    },
    encoding: "utf8",
  });
}

describe("lint-chinese", () => {
  it("passes when scanned TypeScript files contain no Chinese characters", () => {
    const root = createFixture({
      "src/example.ts": 'const message = "Hello";\n',
    });

    const result = runLint(root);

    expect(result.status).toBe(0);
    expect(result.stdout).toContain("Success:");
  });

  it("fails when a scanned TypeScript file contains a Chinese character", () => {
    const chinese = String.fromCharCode(0x4e2d);
    const root = createFixture({
      "src/example.ts": `const message = "${chinese}";\n`,
    });

    const result = runLint(root);

    expect(result.status).toBe(1);
    expect(result.stderr).toContain("Chinese character");
    expect(result.stderr).toContain(`src${path.sep}example.ts:1`);
  });
});

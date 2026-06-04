import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const defaultRootDir = path.resolve(__dirname, "..");
const rootDir = path.resolve(process.env.LINT_CHINESE_ROOT ?? defaultRootDir);

const includeDirs = ["src", "entrypoints"].map((dir) =>
  path.join(rootDir, dir),
);
const includeExtensions = [".ts", ".tsx"];
const chineseCharacterPattern =
  /[\u3000-\u303f\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff\uff00-\uffef]/u;

let hasError = false;

function findChineseCharacter(line) {
  for (const char of line) {
    if (chineseCharacterPattern.test(char)) {
      return char;
    }
  }

  return null;
}

function checkFile(fullPath) {
  if (!includeExtensions.includes(path.extname(fullPath))) return;

  const content = fs.readFileSync(fullPath, "utf8");
  const lines = content.split("\n");

  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i];
    const chineseCharacter = findChineseCharacter(line);

    if (chineseCharacter) {
      hasError = true;
      const relativePath = path.relative(rootDir, fullPath);
      console.error(
        `\x1b[31mError:\x1b[0m Chinese character '\x1b[33m${chineseCharacter}\x1b[0m' found in ${relativePath}:${
          i + 1
        }`,
      );
      console.error(`  > ${line.trim()}`);
    }
  }
}

function checkDir(dir) {
  if (!fs.existsSync(dir)) return;
  const entries = fs.readdirSync(dir, { withFileTypes: true });

  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      checkDir(fullPath);
    } else if (entry.isFile()) {
      checkFile(fullPath);
    }
  }
}

includeDirs.forEach(checkDir);

const rootEntries = fs.readdirSync(rootDir, { withFileTypes: true });
for (const entry of rootEntries) {
  if (entry.isFile()) {
    checkFile(path.join(rootDir, entry.name));
  }
}

if (hasError) {
  console.error(
    `\x1b[31mLint failed:\x1b[0m Chinese characters are not allowed in extension source files.`,
  );
  process.exit(1);
} else {
  console.log(
    `\x1b[32mSuccess:\x1b[0m All scanned extension source files contain no Chinese characters.`,
  );
  process.exit(0);
}

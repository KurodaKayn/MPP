import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const rootDir = path.resolve(__dirname, '..');

const includeDirs = ['src'].map(dir => path.join(rootDir, dir));
const includeExtensions = ['.ts', '.tsx'];

let hasError = false;

function checkAscii(dir) {
    if (!fs.existsSync(dir)) return;
    const entries = fs.readdirSync(dir, { withFileTypes: true });

    for (const entry of entries) {
        const fullPath = path.join(dir, entry.name);
        
        if (entry.isDirectory()) {
            checkAscii(fullPath);
        } else if (entry.isFile()) {
            if (includeExtensions.includes(path.extname(fullPath))) {
                const content = fs.readFileSync(fullPath, 'utf8');
                const lines = content.split('\n');
                
                for (let i = 0; i < lines.length; i++) {
                    const line = lines[i];
                    // Check for any character outside standard ASCII range (0-127)
                    let nonAsciiChar = null;
                    for (let c = 0; c < line.length; c++) {
                        if (line.charCodeAt(c) > 127) {
                            nonAsciiChar = line[c];
                            break;
                        }
                    }
                    if (nonAsciiChar) {
                        hasError = true;
                        const char = nonAsciiChar;
                        const relativePath = path.relative(rootDir, fullPath);
                        console.error(`\x1b[31mError:\x1b[0m Non-ASCII character '\x1b[33m${char}\x1b[0m' found in ${relativePath}:${i + 1}`);
                        console.error(`  > ${line.trim()}`);
                    }
                }
            }
        }
    }
}

includeDirs.forEach(checkAscii);

if (hasError) {
    console.error(`\x1b[31mLint failed:\x1b[0m Non-ASCII characters are not allowed to enforce i18n.`);
    process.exit(1);
} else {
    console.log(`\x1b[32mSuccess:\x1b[0m All scanned files contain only ASCII characters.`);
    process.exit(0);
}

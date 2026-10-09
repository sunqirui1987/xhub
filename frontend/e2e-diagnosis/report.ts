import fs from "fs";
import path from "path";

const dir = path.resolve(__dirname, "../.e2e-report");

function append(name: string, line: string) {
  fs.mkdirSync(dir, { recursive: true });
  fs.appendFileSync(path.join(dir, name), line.endsWith("\n") ? line : `${line}\n`);
}

export function recordPage(route: string, result: "pass" | "fail") {
  append("pages.txt", `page ${result} ${route}`);
}

export function recordChain(line: string) {
  append("chains.txt", line);
}

export const reportDir = dir;

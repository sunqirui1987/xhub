import fs from "fs";
import path from "path";
import { discoverPages } from "./routes";
import { reportDir } from "./report";

function readLines(name: string): string[] {
  const file = path.join(reportDir, name);
  if (!fs.existsSync(file)) return [];
  return fs
    .readFileSync(file, "utf8")
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}

export default function globalTeardown() {
  const pages = new Map<string, string>();
  for (const line of readLines("pages.txt")) {
    const [, result, route] = line.split(" ");
    if (route) pages.set(route, result);
  }
  const discovered = discoverPages();
  const missing = discovered.filter((item) => !pages.has(item.route)).map((item) => item.route);
  const routes = readLines("routes.txt");
  const routeFails = routes.filter((line) => line.startsWith("route fail")).length;
  const chains = readLines("chains.txt");
  const body = [
    "console routes",
    ...discovered.map((item) => `page ${pages.get(item.route) || "missing"} ${item.route}`),
    `console_routes=${discovered.length}`,
    "",
    "catalog routes",
    ...routes,
    `catalog_routes=${routes.length}`,
    `misaligned=${routeFails}`,
    "",
    ...chains,
    "",
  ].join("\n");
  const out = path.resolve(__dirname, "../../e2e/e2e-report.txt");
  fs.writeFileSync(out, body);
  if (process.env.E2E_FULL_COVERAGE === "1" && missing.length > 0) {
    throw new Error(`report missing console routes: ${missing.join(", ")}`);
  }
  if (process.env.E2E_FULL_COVERAGE === "1" && !routes.length) {
    throw new Error("report has no catalog routes");
  }
}

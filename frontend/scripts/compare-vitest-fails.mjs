#!/usr/bin/env node
/**
 * Compare XHub vitest JSON reporter output against the checked-in LiteLLM 1.102.0
 * fail snapshot. Extra or missing failed titles/files fail this check.
 *
 * Usage:
 *   node scripts/compare-vitest-fails.mjs --xhub-dir DIR --snapshot PATH
 */
import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";

function arg(name, fallback) {
  const i = process.argv.indexOf(name);
  return i >= 0 ? process.argv[i + 1] : fallback;
}

function extractFails(report) {
  const out = [];
  for (const suite of report.testResults ?? []) {
    const file = suite.name ?? "";
    const assertions = suite.assertionResults ?? [];
    for (const a of assertions) {
      if (a.status === "failed") {
        out.push({ file, title: a.fullName || a.title || "" });
      }
    }
    if (suite.status === "failed" && assertions.length === 0) {
      const msg = String(suite.message || "").split("\n")[0] ?? "";
      out.push({ file, title: `[suite] ${msg}` });
    }
  }
  out.sort((a, b) => a.file.localeCompare(b.file) || a.title.localeCompare(b.title));
  return out;
}

function key(row) {
  return `${row.file}::${row.title}`;
}

function loadReport(path) {
  return JSON.parse(readFileSync(path, "utf8"));
}

const xhubDir = resolve(arg("--xhub-dir", ""));
const snapshotPath = resolve(arg("--snapshot", ""));
const outPath = arg("--out", "");
const projects = ["unit", "component", "integration", "types"];

if (!xhubDir || !snapshotPath) {
  console.error("usage: compare-vitest-fails.mjs --xhub-dir DIR --snapshot PATH [--out FILE]");
  process.exit(2);
}

const snapshot = JSON.parse(readFileSync(snapshotPath, "utf8"));
const summary = [];
let mismatch = false;

for (const proj of projects) {
  const report = loadReport(resolve(xhubDir, `${proj}.json`));
  const xFails = extractFails(report);
  const lFails = snapshot[proj]?.fails ?? [];
  const xKeys = new Set(xFails.map(key));
  const lKeys = new Set(lFails.map(key));
  const extra = [...xKeys].filter((k) => !lKeys.has(k)).sort();
  const missing = [...lKeys].filter((k) => !xKeys.has(k)).sort();
  const row = {
    project: proj,
    litellmPassed: snapshot[proj]?.numPassedTests ?? null,
    litellmFailed: snapshot[proj]?.numFailedTests ?? lFails.length,
    xhubPassed: report.numPassedTests ?? null,
    xhubFailed: report.numFailedTests ?? xFails.length,
    failSetsEqual: extra.length === 0 && missing.length === 0,
    extra,
    missing,
  };
  if (!row.failSetsEqual) mismatch = true;
  summary.push(row);
  console.log(
    `${proj}: LiteLLM passed=${row.litellmPassed} failed=${row.litellmFailed}; XHub passed=${row.xhubPassed} failed=${row.xhubFailed}; failSetsEqual=${row.failSetsEqual}`,
  );
  if (extra.length) console.log("  extra XHub fails:", extra);
  if (missing.length) console.log("  missing vs LiteLLM:", missing);
}

if (outPath) {
  writeFileSync(outPath, JSON.stringify({ mismatch, summary }, null, 2) + "\n");
}

if (mismatch) {
  console.error("FAIL: vitest fail sets do not match LiteLLM 1.102.0 snapshot");
  process.exit(1);
}
console.log("PASS: fail title/file sets match LiteLLM 1.102.0");

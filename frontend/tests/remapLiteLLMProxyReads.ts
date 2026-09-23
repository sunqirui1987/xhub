import { copyFileSync, mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

/**
 * LiteLLM dashboard tests read proxy JSON via monorepo-relative paths
 * (`../../../../litellm/proxy/...` from `tests/mocks`). XHub is not that
 * monorepo; materialize the same files at that resolved location from vendored
 * fixtures so the shipped tests keep their original paths.
 */
const testsDir = dirname(fileURLToPath(import.meta.url));
const fixtureDir = resolve(testsDir, "fixtures");
const destRoot = resolve(testsDir, "../../../litellm/proxy");

mkdirSync(resolve(destRoot, "public_endpoints"), { recursive: true });
copyFileSync(
  resolve(fixtureDir, "autorouter_presets.json"),
  resolve(destRoot, "public_endpoints/autorouter_presets.json"),
);
copyFileSync(resolve(fixtureDir, "mcp_registry.json"), resolve(destRoot, "mcp_registry.json"));

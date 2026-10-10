import path from "path";
import { defineConfig, devices } from "@playwright/test";
const uiPort = process.env.E2E_UI_PORT || "3100";
const gatewayPort = process.env.E2E_GW_PORT || "4100";
const upstreamPort = process.env.E2E_UP_PORT || "4110";
const gateway = "http://127.0.0.1:" + gatewayPort;
const runDir = path.resolve(__dirname, "../.e2e/current");
Object.assign(process.env, {
  E2E_GATEWAY: gateway, E2E_UPSTREAM: "http://127.0.0.1:" + upstreamPort,
  E2E_RUN_DIR: runDir, E2E_GW_PORT: gatewayPort, E2E_UP_PORT: upstreamPort,
});
export default defineConfig({
  testDir: "./e2e", globalSetup: "./e2e/global-setup.ts", globalTeardown: "./e2e/global-teardown.ts",
  timeout: 60_000, expect: { timeout: 15_000 },
  fullyParallel: false, workers: 1, retries: 0, forbidOnly: !!process.env.CI,
  reporter: [["./e2e/checklist-reporter.cjs"], ["html", { open: "never" }],
    ["json", { outputFile: path.join(runDir, "results.json") }],
    ["junit", { outputFile: path.join(runDir, "junit.xml") }]],
  use: { baseURL: "http://127.0.0.1:" + uiPort, screenshot: "only-on-failure",
    trace: "retain-on-failure", video: "off", ...devices["Desktop Chrome"] },
  webServer: [
    { command: "bash ../e2e/start-gateway.sh", url: gateway + "/health/liveliness",
      reuseExistingServer: false, timeout: 180_000 },
    { command: "npx next start -p " + uiPort + " -H 127.0.0.1",
      url: "http://127.0.0.1:" + uiPort + "/ui/login/", reuseExistingServer: false, timeout: 180_000,
      env: { E2E_BUILD_DIR: ".next-e2e", XHUB_GATEWAY_ORIGIN: gateway } },
  ],
});

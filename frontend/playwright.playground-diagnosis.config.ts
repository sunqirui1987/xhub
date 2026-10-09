import { defineConfig, devices } from "@playwright/test";
import path from "node:path";
const dir = path.resolve(__dirname, "../.e2e/playground-diagnosis");
Object.assign(process.env, { E2E_GATEWAY: "http://127.0.0.1:4200", E2E_UPSTREAM: "http://127.0.0.1:4210", E2E_RUN_DIR: dir });
export default defineConfig({
 testDir: "./e2e-diagnosis", timeout: 120000, expect: { timeout: 20000 }, workers: 1, retries: 0,
 reporter: [["list"], ["json", { outputFile: path.join(dir, "e2e.json") }], ["html", { outputFolder: path.join(dir, "html"), open: "never" }]],
 outputDir: path.join(dir, "browser-results"),
 use: { baseURL: "http://127.0.0.1:3200", screenshot: "only-on-failure", trace: "retain-on-failure", ...devices["Desktop Chrome"] },
});
